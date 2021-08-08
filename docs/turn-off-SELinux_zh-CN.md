# 如何关闭SELinux
## 永久关闭SELinux
```shell script
# 永久关闭SELinux
sed -i 's/SELINUX=enforcing/SELINUX=disabled/g' /etc/selinux/config
#关闭后需要重启系统
reboot
# 查看SELinux的状态
getenforce
```
> 编辑配置文件/etc/selinux/config，把 SELINUX= 更改为 SELINUX=disabled ，然后重启系统，SELinux 就被禁用了

## 临时关闭SELinux
```shell script
# 临时关闭SELinux就是enforcing 和 permissive 两种模式之间进行切换
setenforce 0 #切换成宽容模式
setenforce 1 #切换成强制模式
# check SELinux
getenforce
```
> 临时切换工作模式，重启系统生失效
