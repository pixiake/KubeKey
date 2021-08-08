## Architecture

KubeKey defines task pipelines for cluster operations such as installation, scaling, uninstallation, etc. And uses SSH and Kubernetes Api to perform corresponding tasks on hosts and cluster with host grouping and configuration management.

![Image](img/KubeKey-Architecture.jpg?raw=true)

## Addons
All plugins which are installed by yaml or chart can be kubernetes' addons. So the addons configuration support both yaml and chart.

![Image](img/KubeKey-Addons.jpg?raw=true)

The task of installing KubeSphere is added to the task pipeline of the installation cluster by default. So KubeSphere can be deployed in two ways:

* Using the command `kk create cluster --with-kubesphere`
* Configure KubeSphere as a addon in the configuration file.

> Notice: Installation of KubeSphere using [ks-installer](https://github.com/kubesphere/ks-installer).


## Build Binary from Source Code

### Method 1

```shell script
git clone https://github.com/kubesphere/kubekey.git
cd kubekey
./build.sh
```

> Note:
>
> * Docker needs to be installed before building.
> * If you have problem to access `https://proxy.golang.org/`, excute `build.sh -p` instead.

### Method 2

```shell script
git clone https://github.com/kubesphere/kubekey.git
cd kubekey
make binary
```

> Note:
>
> * Docker needs to be installed before building.
