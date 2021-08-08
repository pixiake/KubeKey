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

package delete

import (
	"bufio"
	"fmt"
	"github.com/kubesphere/kubekey/pkg/util"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/config"
	"github.com/kubesphere/kubekey/pkg/util/executor"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

func ResetCluster(clusterCfgFile string, logger *log.Logger, verbose bool) error {
	cfg, objName, err := config.ParseClusterCfg(clusterCfgFile, "", "", false, logger)
	if err != nil {
		return errors.Wrap(err, "Failed to download cluster config")
	}

	return Execute(executor.NewExecutor(&cfg.Spec, objName, logger, "", verbose, false, true, false, false, nil))
}

func ResetNode(clusterCfgFile string, logger *log.Logger, verbose bool, nodeName string) error {
	if "" == nodeName {
		return errors.New("Node name does not exist")
	}
	fp, _ := filepath.Abs(clusterCfgFile)
	cmd0 := fmt.Sprintf("cat %s | grep %s | wc -l", fp, nodeName)
	nodeNameNum, err0 := exec.Command("/bin/sh", "-c", cmd0).CombinedOutput()
	if err0 != nil {
		return errors.Wrap(err0, "Failed to get node num")
	}
	if string(nodeNameNum) == "2\n" {
		cmd := fmt.Sprintf("sed -i /%s/d %s", nodeName, fp)
		_ = exec.Command("/bin/sh", "-c", cmd).Run()
		cfg, objName, _ := config.ParseClusterCfg(clusterCfgFile, "", "", false, logger)
		return Execute1(executor.NewExecutor(&cfg.Spec, objName, logger, "", verbose, false, true, false, false, nil))
	} else if string(nodeNameNum) == "1\n" {
		cmd := fmt.Sprintf("sed -i /%s/d %s", nodeName, fp)
		_ = exec.Command("/bin/sh", "-c", cmd).Run()
		cfg, objName, err := config.ParseClusterCfg(clusterCfgFile, "", "", false, logger)
		if err != nil {
			return errors.Wrap(err, "Failed to download cluster config")
		}
		mgr, err1 := executor.NewExecutor(&cfg.Spec, objName, logger, "", verbose, false, true, false, false, nil).CreateManager()
		if err1 != nil {
			return errors.Wrap(err1, "Failed to get cluster config")
		}
		var newNodeName []string
		for i := 0; i < len(mgr.WorkerNodes); i++ {
			nodename := mgr.WorkerNodes[i].Name
			if nodeName == nodename {
				continue
			} else {
				newNodeName = append(newNodeName, nodename)
			}
		}
		var connNodeName []string
		for j := 0; j < len(newNodeName); j++ {
			t := j
			nodename1 := newNodeName[t]
			for t+1 < len(newNodeName) && Isadjoin(newNodeName[t], newNodeName[t+1]) {
				t++
			}
			if t == j {
				connNodeName = append(connNodeName, nodename1)
			} else {
				connNodeName = append(connNodeName, Merge(nodename1, newNodeName[t]))
				j = t
			}
		}
		cmd1 := fmt.Sprintf("sed -i -n '1,/worker/p;/controlPlaneEndpoint/,$p' %s", fp)
		_ = exec.Command("/bin/sh", "-c", cmd1).Run()
		for k := 0; k < len(connNodeName); k++ {
			workPar := connNodeName[k]
			workPar1 := fmt.Sprintf("%s", workPar)
			cmd2 := fmt.Sprintf("sed -i '/worker/a\\ \\ \\ \\ \\- %s' %s", workPar1, fp)
			_ = exec.Command("/bin/sh", "-c", cmd2).Run()
		}
		cfg1, objName, _ := config.ParseClusterCfg(clusterCfgFile, "", "", false, logger)
		return Execute1(executor.NewExecutor(&cfg1.Spec, objName, logger, "", verbose, false, true, false, false, nil))
	} else {
		fmt.Println("Please check the node name in the config-sample.yaml or do not support to delete master")
		os.Exit(0)
	}

	return nil
}
func Execute(executor *executor.Executor) error {
	mgr, err := executor.CreateManager()
	if err != nil {
		return err
	}
	return ExecTasks(mgr)
}
func Execute1(executor *executor.Executor) error {
	mgr, err := executor.CreateManager()
	if err != nil {
		return err
	}
	return ExecTasks1(mgr)
}
func ExecTasks(mgr *manager.Manager) error {
	resetTasks := []manager.Task{
		{Task: ResetKubeCluster, ErrMsg: "Failed to reset kube cluster"},
	}

	for _, step := range resetTasks {
		if err := step.Run(mgr); err != nil {
			return errors.Wrap(err, step.ErrMsg)
		}
	}

	mgr.Logger.Infoln("Successful.")

	return nil
}
func ExecTasks1(mgr *manager.Manager) error {
	resetNodeTasks := []manager.Task{
		{Task: ResetKubeNode, ErrMsg: "Failed to reset kube cluster"},
	}

	for _, step := range resetNodeTasks {
		if err := step.Run(mgr); err != nil {
			return errors.Wrap(err, step.ErrMsg)
		}
	}

	mgr.Logger.Infoln("Successful.")

	return nil
}

func ResetKubeCluster(mgr *manager.Manager) error {
	reader := bufio.NewReader(os.Stdin)
	input, err := Confirm(reader)
	if err != nil {
		return err
	}
	if input == "no" {
		os.Exit(0)
	}

	mgr.Logger.Infoln("Resetting kubernetes cluster ...")

	return mgr.RunTaskOnK8sNodes(resetKubeCluster, true)
}
func ResetKubeNode(mgr *manager.Manager) error {
	reader := bufio.NewReader(os.Stdin)
	input, err := Confirm1(reader)
	if err != nil {
		return err
	}
	if input == "no" {
		os.Exit(0)
	}

	mgr.Logger.Infoln("Resetting kubernetes node ...")

	return mgr.RunTaskOnMasterNodes(resetKubeNode, true)
}

func resetKubeNode(mgr *manager.Manager, _ *kubekeyapiv1alpha1.HostCfg) error {
	if mgr.Runner.Index == 0 {
		var deletenodename string
		var tmp []string
		output1, _ := mgr.Runner.ExecuteCmd("sudo -E  /usr/local/bin/kubectl get nodes | grep -v NAME | grep -v 'master' | awk '{print $1}'", 0, true)
		if !strings.Contains(output1, "\r\n") {
			tmp = append(tmp, output1)
			fmt.Println("")
			os.Exit(0)
		} else {
			tmp = strings.Split(output1, "\r\n")
		}
		var tmp1 string
		for j := 0; j < len(mgr.WorkerNodes); j++ {
			tmp1 += mgr.WorkerNodes[j].Name + "\r\n"
		}
		for i := 0; i < len(tmp); i++ {
			if strings.Contains(tmp1, tmp[i]) {
				continue
			} else {
				deletenodename = tmp[i]
				break
			}
		}
		if err := DrainAndDeleteNode(mgr, deletenodename); err != nil {
			return err
		}
	}
	return nil
}
func DrainAndDeleteNode(mgr *manager.Manager, deleteNodeName string) error {
	_, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"/usr/local/bin/kubectl drain %s --delete-local-data --ignore-daemonsets\"", deleteNodeName), 5, true)
	if err != nil {
		return errors.Wrap(err, "Failed to drain the node")
	}
	_, err1 := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"/usr/local/bin/kubectl delete node %s\"", deleteNodeName), 5, true)
	if err1 != nil {
		return errors.Wrap(err1, "Failed to delete the node")
	}
	return nil
}
func Merge(name1, name2 string) (endname string) {
	par1, par2 := SplitNum(name1)
	_, par4 := SplitNum(name2)
	var endName string
	endName = fmt.Sprintf("%s[%s:%s]", par1, strconv.Itoa(par2), strconv.Itoa(par4))
	return endName
}
func Isadjoin(name1, name2 string) bool {
	Isad := false
	par1, par2 := SplitNum(name1)
	par3, par4 := SplitNum(name2)
	if par1 == par3 && par4 == par2+1 {
		Isad = true
	}
	return Isad
}

func SplitNum(nodename string) (name string, num int) {
	nodelen := len(nodename)
	i := nodelen - 1
	for ; nodelen > 0; i-- {
		if !unicode.IsDigit(rune(nodename[i])) {
			num, _ := strconv.Atoi(nodename[i+1:])
			name := nodename[:i+1]
			return name, num
		}
	}
	return "", 0
}

var (
	clusterFiles = []string{
		"/usr/local/bin/etcd",
		"/etc/ssl/etcd",
		"/var/lib/etcd",
		"/etc/etcd.env",
		"/etc/kubernetes",
		"/etc/systemd/system/etcd.service",
		"/var/log/calico",
		"/etc/cni",
		"/var/log/pods/",
		"/var/lib/cni",
		"/var/lib/calico",
		"/var/lib/kubelet",
		"/run/calico",
		"/run/flannel",
		"/etc/flannel",
		"/var/openebs",
		"/etc/systemd/system/kubelet.service",
		"/etc/systemd/system/kubelet.service.d",
		"/usr/local/bin/kubelet",
		"/usr/local/bin/kubeadm",
		"/usr/bin/kubelet",
		"/var/lib/rook",
	}

	kubeovnFiles = []string{
		"/var/run/openvswitch",
		"/var/run/ovn",
		"/etc/openvswitch",
		"/etc/ovn",
		"/var/log/openvswitch",
		"/var/log/ovn",
		"/etc/cni/net.d/00-kube-ovn.conflist",
		"/etc/cni/net.d/01-kube-ovn.conflist",
	}

	cmdsList = []string{
		"iptables -F",
		"iptables -X",
		"iptables -F -t nat",
		"iptables -X -t nat",
		"ip link del kube-ipvs0",
		"ip link del nodelocaldns",
	}
)

func resetKubeCluster(mgr *manager.Manager, _ *kubekeyapiv1alpha1.HostCfg) error {
	// delete OVN/OVS DB and config files on every node
	if mgr.Cluster.Network.Plugin == "kubeovn" {
		deleteOvnFiles(mgr)
	}

	switch mgr.Cluster.Kubernetes.Type {
	case "k3s":
		_, _ = mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"systemctl daemon-reload && /usr/local/bin/k3s-killall.sh\"", 0, true)
		_, _ = mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"systemctl daemon-reload && /usr/local/bin/k3s-uninstall.sh\"", 0, true)
	default:
		if util.IsExist("/usr/local/bin/k3s-uninstall.sh") {
			_, _ = mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"systemctl daemon-reload && /usr/local/bin/k3s-killall.sh\"", 0, true)
			_, _ = mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"systemctl daemon-reload && /usr/local/bin/k3s-uninstall.sh\"", 0, true)
		} else {
			_, _ = mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/local/bin/kubeadm reset -f\"", 0, true)
			_, _ = mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"%s\"", strings.Join(cmdsList, " && ")), 0, true, "printCmd")
		}
	}
	_ = deleteFiles(mgr)
	return nil
}

func deleteFiles(mgr *manager.Manager) error {
	_, _ = mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"systemctl stop etcd && exit 0\"", 0, true)

	for _, file := range clusterFiles {
		_, _ = mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"rm -rf %s\"", file), 0, false)
	}
	_, _ = mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"systemctl daemon-reload && exit 0\"", 0, true)
	return nil
}

func deleteOvnFiles(mgr *manager.Manager) {
	_, _ = mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"/usr/share/openvswitch/scripts/ovs-ctl stop && ovs-dpctl del-dp ovs-system\"", 1, true)
	for _, file := range kubeovnFiles {
		_, _ = mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"rm -rf %s\"", file), 1, true)
	}
}

func Confirm(reader *bufio.Reader) (string, error) {
	for {
		fmt.Printf("Are you sure to delete this cluster? [yes/no]: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		input = strings.TrimSpace(input)

		if input != "" && (input == "yes" || input == "no") {
			return input, nil
		}
	}
}

func Confirm1(reader *bufio.Reader) (string, error) {
	for {
		fmt.Printf("Are you sure to delete this node? [yes/no]: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		input = strings.TrimSpace(input)

		if input != "" && (input == "yes" || input == "no") {
			return input, nil
		}
	}
}
