/*
Copyright 2020 The KubeSphere Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubekey

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"text/template"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	kubekeyclientset "github.com/kubesphere/kubekey/clients/clientset/versioned"
	"github.com/kubesphere/kubekey/pkg/addons/manifests"
	"github.com/kubesphere/kubekey/pkg/util"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/lithammer/dedent"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	kubeErr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kube "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Pending representative the cluster is being created.
	Pending = "pending"
	// Success representative the cluster cluster has been created successfully.
	Success = "success"
	// Failed creation failure.
	Failed = "failed"
)

var (
	newNodes          []string
	clusterKubeSphere = template.Must(template.New("cluster.kubesphere.io").Parse(
		dedent.Dedent(`apiVersion: cluster.kubesphere.io/v1alpha1
kind: Cluster
metadata:
  finalizers:
  - finalizer.cluster.kubesphere.io
  labels:
    cluster-role.kubesphere.io/member: ""
    kubesphere.io/managed: "true"
    kubekey.kubesphere.io/name: {{ .Name }}
  name: {{ .Name }} 
spec:
  connection:
    kubeconfig: {{ .Kubeconfig }}
    type: direct
  enable: {{ .Enable }}
  joinFederation: {{ .JoinFedration }}
  provider: kubesphere
    `)))
)

func generateClusterKubeSphere(name string, kubeconfig string, enable, joinFedration bool) (string, error) {
	return util.Render(clusterKubeSphere, util.Data{
		"Name":          name,
		"Kubeconfig":    kubeconfig,
		"Enable":        enable,
		"JoinFedration": joinFedration,
	})
}

// CheckClusterRole is used to check the cluster's role (host or member).
func CheckClusterRole() (bool, *rest.Config, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return false, nil, err
	}
	// creates the clientset
	clientset, err := kube.NewForConfig(config)
	if err != nil {
		return false, nil, err
	}
	var hostClusterFlag bool
	if err := clientset.RESTClient().Get().
		AbsPath("/apis/cluster.kubesphere.io/v1alpha1/clusters").
		Name("host").
		Do(context.TODO()).Error(); err == nil {
		hostClusterFlag = true
	}
	return hostClusterFlag, config, nil
}

func newKubeSphereCluster(r *ClusterReconciler, c *kubekeyapiv1alpha1.Cluster) error {
	if hostClusterFlag, config, err := CheckClusterRole(); err != nil {
		return err
	} else if hostClusterFlag {
		obj, err := generateClusterKubeSphere(c.Name, "Cg==", false, false)
		if err != nil {
			return err
		}
		if err := manifests.DoServerSideApply(context.TODO(), config, []byte(obj)); err != nil {
			_ = r.Delete(context.TODO(), c)
			return err
		}

		kscluster, err1 := manifests.GetCluster(c.Name)
		if err1 != nil {
			return err
		}
		ownerReferencePatch := fmt.Sprintf(`{"metadata": {"ownerReferences": [{"apiVersion": "%s", "kind": "%s", "name": "%s", "uid": "%s"}]}}`, kscluster.GetAPIVersion(), kscluster.GetKind(), kscluster.GetName(), kscluster.GetUID())
		if err := r.Patch(context.TODO(), c, client.RawPatch(types.MergePatchType, []byte(ownerReferencePatch))); err != nil {
			return err
		}
	}
	return nil
}

// UpdateKubeSphereCluster is used to update the cluster object of KubeSphere's multicluster.
func UpdateKubeSphereCluster(mgr *manager.Manager) error {
	if hostClusterFlag, config, err := CheckClusterRole(); err != nil {
		return err
	} else if hostClusterFlag {
		obj, err := generateClusterKubeSphere(mgr.ObjName, mgr.Kubeconfig, true, true)
		if err != nil {
			return err
		}
		if err := manifests.DoServerSideApply(context.TODO(), config, []byte(obj)); err != nil {
			return err
		}
	}
	return nil
}

// NewKubekeyClient is used to create a kubekey cluster client.
func NewKubekeyClient() (*kubekeyclientset.Clientset, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	// creates the clientset
	clientset := kubekeyclientset.NewForConfigOrDie(config)

	return clientset, nil
}

func getCluster(name string) (*kubekeyapiv1alpha1.Cluster, error) {
	clientset, err := NewKubekeyClient()
	if err != nil {
		return nil, err
	}
	clusterObj, err := clientset.KubekeyV1alpha1().Clusters().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return clusterObj, nil
}

// UpdateClusterConditions is used for updating cluster installation process information or adding nodes.
func UpdateClusterConditions(mgr *manager.Manager, step string, startTime, endTime metav1.Time, status bool, index int) error {
	condition := kubekeyapiv1alpha1.Condition{
		Step:      step,
		StartTime: startTime,
		EndTime:   endTime,
		Status:    status,
	}
	if len(mgr.Conditions) < index {
		mgr.Conditions = append(mgr.Conditions, condition)
	} else if len(mgr.Conditions) == index {
		mgr.Conditions[index-1] = condition
	}

	cluster, err := getCluster(mgr.ObjName)
	if err != nil {
		return err
	}

	cluster.Status.Conditions = mgr.Conditions

	if _, err := mgr.ClientSet.KubekeyV1alpha1().Clusters().UpdateStatus(context.TODO(), cluster, metav1.UpdateOptions{}); err != nil {
		return err
	}
	return nil
}

// UpdateStatus is used to update status for a new or expanded cluster.
func UpdateStatus(mgr *manager.Manager) error {
	cluster, err := getCluster(mgr.ObjName)
	if err != nil {
		return err
	}
	cluster.Status.Version = mgr.Cluster.Kubernetes.Version
	cluster.Status.NodesCount = len(mgr.AllNodes)
	cluster.Status.MasterCount = len(mgr.MasterNodes)
	cluster.Status.WorkerCount = len(mgr.WorkerNodes)
	cluster.Status.EtcdCount = len(mgr.EtcdNodes)
	cluster.Status.NetworkPlugin = mgr.Cluster.Network.Plugin
	cluster.Status.Nodes = []kubekeyapiv1alpha1.NodeStatus{}

	for _, node := range mgr.AllNodes {
		cluster.Status.Nodes = append(cluster.Status.Nodes, kubekeyapiv1alpha1.NodeStatus{
			InternalIP: node.InternalAddress,
			Hostname:   node.Name,
			Roles:      map[string]bool{"etcd": node.IsEtcd, "master": node.IsMaster, "worker": node.IsWorker},
		})
	}

	if _, err := mgr.ClientSet.KubekeyV1alpha1().Clusters().UpdateStatus(context.TODO(), cluster, metav1.UpdateOptions{}); err != nil {
		return err
	}
	return nil
}

func getClusterClientSet(mgr *manager.Manager) (*kube.Clientset, error) {
	// creates the in-cluster config
	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	// creates the clientset
	clientset, err := kube.NewForConfig(inClusterConfig)
	if err != nil {
		return nil, err
	}

	obj, err := clientset.RESTClient().Get().AbsPath("/apis/cluster.kubesphere.io/v1alpha1/clusters").Name(mgr.ObjName).Do(context.TODO()).Raw()
	if err != nil && !kubeErr.IsNotFound(err) {
		return nil, err
	} else if kubeErr.IsNotFound(err) {
		return nil, nil
	}

	result := make(map[string]interface{})
	_ = yaml.Unmarshal(obj, &result)

	spec := result["spec"].(map[interface{}]interface{})
	connection := spec["connection"].(map[interface{}]interface{})

	kubeconfigStr, _ := base64.StdEncoding.DecodeString(connection["kubeconfig"].(string))

	kubeConfigPath := filepath.Join(mgr.WorkDir, fmt.Sprintf("config-%s", mgr.ObjName))
	if err := ioutil.WriteFile(kubeConfigPath, kubeconfigStr, 0644); err != nil {
		return nil, err
	}
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, err
	}
	clientsetForCluster, err := kube.NewForConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	return clientsetForCluster, nil
}

func nodeForCluster(name string, labels map[string]string) *corev1.Node {
	node := &corev1.Node{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec:   corev1.NodeSpec{},
		Status: corev1.NodeStatus{},
	}

	return node
}

// CreateNodeForCluster is used to create new nodes for the cluster to be add nodes.
func CreateNodeForCluster(mgr *manager.Manager) error {
	clientsetForCluster, err := getClusterClientSet(mgr)
	if err != nil && !kubeErr.IsNotFound(err) {
		return err
	} else if kubeErr.IsNotFound(err) {
		return nil
	}
	nodeInfo := make(map[string]string)
	nodeList, _ := clientsetForCluster.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		nodeInfo[node.Name] = node.Status.NodeInfo.KubeletVersion
	}

	for _, host := range mgr.K8sNodes {
		if _, ok := nodeInfo[host.Name]; !ok {
			labels := map[string]string{"kubekey.kubesphere.io/import-status": Pending}
			if host.IsMaster {
				labels["node-role.kubernetes.io/master"] = ""
			}
			if host.IsWorker {
				labels["node-role.kubernetes.io/worker"] = ""
			}
			node := nodeForCluster(host.Name, labels)
			if _, err = clientsetForCluster.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{}); err != nil {
				return err
			}
			newNodes = append(newNodes, host.Name)
		}
	}

	return nil
}

// PatchNodeImportStatus is used to update new node's status.
func PatchNodeImportStatus(mgr *manager.Manager, status string) error {
	clientsetForCluster, err := getClusterClientSet(mgr)
	if err != nil && !kubeErr.IsNotFound(err) {
		return err
	} else if kubeErr.IsNotFound(err) {
		return nil
	}

	patchStr := fmt.Sprintf(`{"metadata": {"labels": {"kubekey.kubesphere.io/import-status": "%s"}}}`, status)
	nodeList, _ := clientsetForCluster.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		for k, v := range node.Labels {
			if k == "kubekey.kubesphere.io/import-status" && v != Success {
				_, err = clientsetForCluster.CoreV1().Nodes().Patch(context.TODO(), node.Name, types.StrategicMergePatchType, []byte(patchStr), metav1.PatchOptions{})
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// SaveKubeConfig is used to save the kubeconfig for the new cluster.
func SaveKubeConfig(mgr *manager.Manager) error {
	// creates the in-cluster config
	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return err
	}
	// creates the clientset
	clientset, err := kube.NewForConfig(inClusterConfig)
	if err != nil {
		return err
	}
	cmClientset := clientset.CoreV1().ConfigMaps("kubekey-system")

	if _, err := cmClientset.Get(context.TODO(), fmt.Sprintf("%s-kubeconfig", mgr.ObjName), metav1.GetOptions{}); err != nil {
		if kubeErr.IsNotFound(err) {
			cmClientset.Create(context.TODO(), configMapForKubeconfig(mgr), metav1.CreateOptions{})
		} else {
			return err
		}
	} else {
		kubeconfigStr := fmt.Sprintf(`{"kubeconfig": "%s"}`, mgr.Kubeconfig)
		cmClientset.Patch(context.TODO(), mgr.ObjName, types.ApplyPatchType, []byte(kubeconfigStr), metav1.PatchOptions{})
	}
	// clientset.CoreV1().ConfigMaps("kubekey-system").Create(context.TODO(), kubeconfigConfigMap, metav1.CreateOptions{}
	return nil
}

// configMapForKubeconfig is used to generate configmap scheme for cluster's kubeconfig.
func configMapForKubeconfig(mgr *manager.Manager) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-kubeconfig", mgr.ObjName),
		},
		Data: map[string]string{
			"kubeconfig": mgr.Kubeconfig,
		},
	}

	return cm
}
