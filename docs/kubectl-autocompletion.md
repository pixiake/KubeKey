Enable kubectl autocompletion
------------

KubeKey doesn't enable kubectl autocompletion. Refer to the guide below and turn it on:

**Prerequisite**: make sure bash-autocompletion is installed and works.

```shell script
# Install bash-completion
apt-get install bash-completion

# Source the completion script in your ~/.bashrc file
echo 'source <(kubectl completion bash)' >>~/.bashrc

# Add the completion script to the /etc/bash_completion.d directory
kubectl completion bash >/etc/bash_completion.d/kubectl
```

More detail reference could be found [here](https://kubernetes.io/docs/tasks/tools/install-kubectl/#enabling-shell-autocompletion).