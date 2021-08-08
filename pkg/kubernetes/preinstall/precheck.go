package preinstall

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	kubekeyapiv1alpha1 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha1"
	"github.com/kubesphere/kubekey/pkg/util/manager"
	"github.com/mitchellh/mapstructure"
	"github.com/modood/table"
)

// PrecheckResults defines the items to be checked.
type PrecheckResults struct {
	Name      string `table:"name"`
	Sudo      string `table:"sudo"`
	Curl      string `table:"curl"`
	Openssl   string `table:"openssl"`
	Ebtables  string `table:"ebtables"`
	Socat     string `table:"socat"`
	Ipset     string `table:"ipset"`
	Conntrack string `table:"conntrack"`
	Docker    string `table:"docker"`
	Nfs       string `table:"nfs client"`
	Ceph      string `table:"ceph client"`
	Glusterfs string `table:"glusterfs client"`
	Time      string `table:"time"`
}

var (
	// CheckResults is used to save save check results.
	CheckResults = make(map[string]interface{})
	// BaseSoftwares defines the software to be checked.
	BaseSoftwares = []string{"sudo", "curl", "openssl", "ebtables", "socat", "ipset", "conntrack", "docker", "showmount", "rbd", "glusterfs"}
)

// PrecheckNodes is used to check nodes before installation.
func PrecheckNodes(mgr *manager.Manager, node *kubekeyapiv1alpha1.HostCfg) error {
	var results = make(map[string]interface{})
	results["name"] = node.Name
	for _, software := range BaseSoftwares {
		_, err := mgr.Runner.ExecuteCmd(fmt.Sprintf("sudo -E /bin/sh -c \"which %s\"", software), 0, false)
		switch software {
		case "showmount":
			software = "nfs"
		case "rbd":
			software = "ceph"
		case "glusterfs":
			software = "glusterfs"
		}
		if err != nil {
			results[software] = ""
		} else {
			results[software] = "y"
			if software == "docker" {
				dockerVersion, err := mgr.Runner.ExecuteCmd("sudo -E /bin/sh -c \"docker version --format '{{.Server.Version}}'\"", 0, false)
				if err == nil {
					results[software] = dockerVersion
				}
			}
		}
	}
	output, err := mgr.Runner.ExecuteCmd("date +\"%Z %H:%M:%S\"", 0, false)
	if err != nil {
		results["time"] = ""
	} else {
		results["time"] = strings.TrimSpace(output)
	}

	CheckResults[node.Name] = results
	return nil
}

// PrecheckConfirm is used to show check results and interact with user.
func PrecheckConfirm(mgr *manager.Manager) {

	var (
		results  []PrecheckResults
		stopFlag bool
	)
	for node := range CheckResults {
		var result PrecheckResults
		_ = mapstructure.Decode(CheckResults[node], &result)
		results = append(results, result)
	}
	table.OutputA(results)
	reader := bufio.NewReader(os.Stdin)

	for _, host := range results {
		if host.Conntrack == "" {
			fmt.Printf("%s: conntrack is required. \n", host.Name)
			stopFlag = true
		}
	}

	if stopFlag {
		os.Exit(1)
	}

	fmt.Println("")
	fmt.Println("This is a simple check of your environment.")
	fmt.Println("Before installation, you should ensure that your machines meet all requirements specified at")
	fmt.Println("https://github.com/kubesphere/kubekey#requirements-and-recommendations")
	fmt.Println("")
Loop:
	for {
		fmt.Printf("Continue this installation? [yes/no]: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			mgr.Logger.Fatal(err)
		}
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "yes":
			break Loop
		case "no":
			os.Exit(0)
		default:
			continue
		}
	}
}
