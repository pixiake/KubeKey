# How to turn off SELinux
## turn off SELinux
```shell script
# Edit the configuration
sed -i 's/SELINUX=enforcing/SELINUX=disabled/g' /etc/selinux/config
#restart the system  
reboot
# check SELinux
getenforce
```
> Edit the configuration file /etc/selinux/config, change SELINUX= to SELINUX=disabled, then restart the system, SELinux will be disabled

## Temporarily shut down SELinux
```shell script
# Temporarily closing SELinux is to switch between enforcing and permissive modes
setenforce 0 #Switch to tolerance mode
setenforce 1 #Switch to mandatory mode
# check SELinux
getenforce
```
> Temporary shutdown enforcing, invalid after restarting the system
