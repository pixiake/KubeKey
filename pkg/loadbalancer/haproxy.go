package loadbalancer

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/kubernetes/preinstall"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/lithammer/dedent"
	"github.com/pkg/errors"
	"io/ioutil"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
)

var haproxyTemplate = template.Must(template.New("haproxy").Parse(
	dedent.Dedent(`
apiVersion: v1
kind: Pod
metadata:
  name: haproxy
  namespace: kube-system
  labels:
    addonmanager.kubernetes.io/mode: Reconcile
    k8s-app: kube-haproxy
  annotations:
    cfg-checksum: "{{ .Checksum }}"
spec:
  hostNetwork: true
  dnsPolicy: ClusterFirstWithHostNet
  nodeSelector:
    kubernetes.io/os: linux
  priorityClassName: system-node-critical
  containers:
  - name: haproxy
    image: {{ .HaproxyImage }}
    imagePullPolicy: Always
    resources:
      requests:
        cpu: 25m
        memory: 32M
    livenessProbe:
      httpGet:
        path: /healthz
        port: {{ .LoadbalancerApiserverHealthcheckPort }}
    readinessProbe:
      httpGet:
        path: /healthz
        port: {{ .LoadbalancerApiserverHealthcheckPort }}
    volumeMounts:
    - mountPath: /usr/local/etc/haproxy/
      name: etc-haproxy
      readOnly: true
  volumes:
  - name: etc-haproxy
    hostPath:
      path: /etc/kubekey/haproxy
`)))

var haproxyConfig = template.Must(template.New("haproxy.cfg").Parse(
	dedent.Dedent(`
global
    maxconn                 4000
    log                     127.0.0.1 local0

defaults
    mode                    http
    log                     global
    option                  httplog
    option                  dontlognull
    option                  http-server-close
    option                  redispatch
    retries                 5
    timeout http-request    5m
    timeout queue           5m
    timeout connect         30s
    timeout client          30s
    timeout server          15m
    timeout http-keep-alive 30s
    timeout check           30s
    maxconn                 4000

frontend healthz
  bind *:{{ .LoadbalancerApiserverHealthcheckPort }}
  mode http
  monitor-uri /healthz

frontend kube_api_frontend
  bind 127.0.0.1:{{ .LoadbalancerApiserverPort }}
  mode tcp
  option tcplog
  default_backend kube_api_backend

backend kube_api_backend
  mode tcp
  balance leastconn
  default-server inter 15s downinter 15s rise 2 fall 2 slowstart 60s maxconn 1000 maxqueue 256 weight 100
  {{- if ne .KubernetesType "k3s"}}
  option httpchk GET /healthz
  {{- end }}
  http-check expect status 200
  {{- range .MasterNodes }}
  server {{ . }} check check-ssl verify none
  {{- end }}
`)))

func GenerateHaproxyManifest(mgr *manager.Manager, checksum string) (string, error) {
	return util.Render(haproxyTemplate, util.Data{
		"HaproxyImage": preinstall.GetImage(mgr, "haproxy").ImageName(),
		// todo: Consider whether need to customize the health check port
		"LoadbalancerApiserverHealthcheckPort": 8081,
		"Checksum":                             checksum,
	})
}

func GenerateHaproxyConf(mgr *manager.Manager) (string, error) {
	masterNodes := make([]string, len(mgr.MasterNodes))
	for i, node := range mgr.MasterNodes {
		masterNodes[i] = node.Name + " " + node.InternalAddress + ":" + strconv.Itoa(mgr.Cluster.ControlPlaneEndpoint.Port)
	}
	return util.Render(haproxyConfig, util.Data{
		"MasterNodes":                          masterNodes,
		"LoadbalancerApiserverPort":            mgr.Cluster.ControlPlaneEndpoint.Port,
		"LoadbalancerApiserverHealthcheckPort": 8081,
		"KubernetesType":                       mgr.Cluster.Kubernetes.Type,
	})
}

func DeployHaproxy(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	if node.IsMaster {
		return nil
	}
	fmt.Printf("[%s] generate haproxy manifest.\n", node.Name)

	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"mkdir -p %s\"", "/etc/kubekey/haproxy"),
		2, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to make internal LB haproxy dir")
	}

	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"chmod 700 %s\"", "/etc/kubekey/haproxy"),
		1, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to chmod internal LB haproxy dir")
	}

	haproxyConf, err := GenerateHaproxyConf(mgr)
	if err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Faield to generate haproxy conf: %s", err))
	}

	if err := ioutil.WriteFile(fmt.Sprintf("%s/haproxy.cfg", mgr.WorkDir), []byte(haproxyConf), 0755); err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to generate internal LB haproxy conf: %s/haproxy.cfg", mgr.WorkDir))
	}

	haproxyConfBase64, err := exec.Command("/bin/bash", "-c",
		fmt.Sprintf("tar cfz - -C %s -T /dev/stdin <<< haproxy.cfg | base64 --wrap=0", mgr.WorkDir)).CombinedOutput()
	if err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to read internal LB haproxy conf")
	}

	if _, err = mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/bash -c \"base64 -d <<< '%s' | tar xz -C %s\"",
		strings.TrimSpace(string(haproxyConfBase64)), "/etc/kubekey/haproxy"), 2, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to generate internal LB haproxy conf")
	}

	if _, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"chmod 755 %s\"", "/etc/kubekey/haproxy/haproxy.cfg"),
		1, false); err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to chmod internal LB haproxy dir")
	}

	// Calculation config md5 as the checksum.
	// It will make load balancer reload when config changes.
	md5Obj := md5.New()
	md5Obj.Write(haproxyConfBase64)
	md5Str := hex.EncodeToString(md5Obj.Sum(nil))

	haproxyManifest, err := GenerateHaproxyManifest(mgr, md5Str)
	if err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Faield to generate haproxy manifest: %s", err))
	}

	if err := ioutil.WriteFile(fmt.Sprintf("%s/haproxy.yaml", mgr.WorkDir), []byte(haproxyManifest), 0644); err != nil {
		return errors.Wrap(errors.WithStack(err), fmt.Sprintf("Failed to generate internal LB haproxy manifests: %s/haproxy.yaml", mgr.WorkDir))
	}

	haproxyBase64, err := exec.Command("/bin/bash", "-c", fmt.Sprintf("tar cfz - -C %s -T /dev/stdin <<< haproxy.yaml | base64 --wrap=0", mgr.WorkDir)).CombinedOutput()
	if err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to read internal LB haproxy manifests")
	}

	var path string
	if mgr.Cluster.Kubernetes.Type == "k3s" {
		path = "/var/lib/rancher/k3s/agent/pod-manifests"
	} else {
		path = "/etc/kubernetes/manifests"
	}
	_, err = mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/bash -c \"base64 -d <<< '%s' | tar xz -C %s\"",
		strings.TrimSpace(string(haproxyBase64)), path), 2, false)
	if err != nil {
		return errors.Wrap(errors.WithStack(err), "Failed to generate internal LB haproxy manifests")
	}
	return nil
}
