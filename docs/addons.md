Addons
------------

All plugins which are installed by yaml or chart can be kubernetes' addons. So the addons configuration support both yaml and chart.

Explanation of parameters:
```yaml
- name: xxx                  # the name of addon
  namespace: xxx             # namespace
  sources:                    # support both yaml and chart
    chart:                          
      name: xxx              # the name of chart
      repo:  xxx             # the name of chart repo (url)
      path: xxx              # the location of chart  (path)
      values:  xxx           # specify values for chart (string list)
      valuesFile: xxx        # specify values file for chart (path / url)
    yaml: 
      path: []               # the location list of yaml (path / url) 
```
example:
```yaml
apiVersion: kubekey.kubesphere.io/v1alpha1
kind: Cluster
metadata:
  name: example
spec:
  hosts:
  - {name: node1, address: 172.16.0.2, internalAddress: 172.16.0.2, privateKeyPath: "~/.ssh/id_rsa"}
  - {name: node2, address: 172.16.0.3, internalAddress: 172.16.0.3, privateKeyPath: "~/.ssh/id_rsa"}
  - {name: node3, address: 172.16.0.4, internalAddress: 172.16.0.4, privateKeyPath: "~/.ssh/id_rsa"}
  ...
  addons:
  - name: nfs-client
    namespace: kube-system
    sources: 
      chart: 
        name: nfs-client-provisioner
        repo: https://charts.kubesphere.io/main
        valuesFile: /mycluster/nfs/custom-nfs-client-values.yaml  # or https://raw.githubusercontent.com/kubesphere/helm-charts/master/src/main/nfs-client-provisioner/values.yaml
        # values also supports parameter lists
        # values:
        # - storageClass.defaultClass=true
        # - nfs.server=192.168.6.3
        # - nfs.path=/mnt/kubesphere
    
  - name: glusterfs
    namespace: kube-system
    sources: 
      yaml: 
        path: 
        - /mycluster/glusterfs/glusterfs.yaml  # or https://raw.githubusercontent.com/xxx/glusterfs.yaml

  - name: sonarqube
    namespace: test
    sources:
      chart:
        name: sonarqube
        repo: https://charts.kubesphere.io/main

  - name: csi-qingcloud
    namespace: kube-system
    sources:
      chart:
        name: csi-qingcloud
        repo: https://charts.kubesphere.io/test
        values:
        - config.qy_access_key_id=***
        - config.qy_secret_access_key=***
        - config.zone=***
        - sc.isDefaultClass=true

  - name: rbd-provisioner
    namespace: kube-system
    sources:
      chart:
        name: rbd-provisioner
        repo: https://charts.kubesphere.io/test
        values:
        # for more values, see https://github.com/kubesphere/helm-charts/tree/master/src/test/rbd-provisioner 
        - ceph.mon=***
        - ceph.pool=***
        - ceph.adminId=***
        - ceph.adminKey=***
        - ceph.userId=***
        - ceph.userKey=***
        - sc.isDefault=true
```
