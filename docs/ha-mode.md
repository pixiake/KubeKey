# HA mode (internal loadbalancing)
K8s components require a loadbalancer to access the apiservers via a reverse proxy. Kubekey provides an internal ha mode. The way kubekey uses is referred to as localhost loadbalancing. The kubelet of each master node connects the local kube-apiserver, and the kubelet of each worker node connects the kube-apiserver via a local reverse proxy. Based on this, kubekey will deploy a haproxy-based proxy that resides on each worker node as the local reverse proxy.

## Usage
Modify your configuration file and uncomment the item `internalLoadbalancer`:
```yaml
controlPlaneEndpoint:
    ##Internal loadbalancer for apiservers
    internalLoadbalancer: haproxy
    
    domain: lb.kubesphere.local
    address: ""
    port: 6443
```

Then whether you exec the command `create cluster`, `add nodes` or `upgrade`, kubekey will enable HA mode and deploy the interanl load balancer. 