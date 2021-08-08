### Certificate
#### Check certificate expiration
```shell script
./kk certs check-expiration [(-f | --file) path]

-f to specify the configuration file which was generated for cluster creation. This parameter is not required if it is single node.

./kk certs check-expiration
INFO[21:33:59 CST] Listing cluster certs ...                    
CERTIFICATE                    EXPIRES                  RESIDUAL TIME   CERTIFICATE AUTHORITY   NODE
apiserver.crt                  Dec 18, 2021 08:27 UTC   352d            ca                      node1   
apiserver-kubelet-client.crt   Dec 18, 2021 08:27 UTC   352d            ca                      node1   
front-proxy-client.crt         Dec 18, 2021 08:27 UTC   352d            front-proxy-ca          node1   
admin.conf                     Dec 18, 2021 08:27 UTC   352d                                    node1   
controller-manager.conf        Dec 18, 2021 08:27 UTC   352d                                    node1   
scheduler.conf                 Dec 18, 2021 08:27 UTC   352d                                    node1   

CERTIFICATE AUTHORITY   EXPIRES                  RESIDUAL TIME   NODE
ca.crt                  Dec 16, 2030 08:27 UTC   9y              node1   
front-proxy-ca.crt      Dec 16, 2030 08:27 UTC   9y              node1   
INFO[21:34:00 CST] Successful. 
```

#### Renew certificate
```shell script
./kk certs renew [(-f | --file) path]

-f to specify the configuration file which was generated for cluster creation. This parameter is not required if it is single node.

./kk certs renew
INFO[21:42:51 CST] Renewing cluster certs ...                   
INFO[21:42:54 CST] Syncing cluster kubeConfig ...               
INFO[21:42:55 CST] Listing cluster certs ...                    
CERTIFICATE                    EXPIRES                  RESIDUAL TIME   CERTIFICATE AUTHORITY   NODE
apiserver.crt                  Dec 30, 2021 13:42 UTC   364d            ca                      node1   
apiserver-kubelet-client.crt   Dec 30, 2021 13:42 UTC   364d            ca                      node1   
front-proxy-client.crt         Dec 30, 2021 13:42 UTC   364d            front-proxy-ca          node1   
admin.conf                     Dec 30, 2021 13:42 UTC   364d                                    node1   
controller-manager.conf        Dec 30, 2021 13:42 UTC   364d                                    node1   
scheduler.conf                 Dec 30, 2021 13:42 UTC   364d                                    node1   

CERTIFICATE AUTHORITY   EXPIRES                  RESIDUAL TIME   NODE
ca.crt                  Dec 16, 2030 08:27 UTC   9y              node1   
front-proxy-ca.crt      Dec 16, 2030 08:27 UTC   9y              node1
```
