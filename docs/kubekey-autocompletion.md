Enable kubekey auto-completion
------------

**Prerequisite**: make sure `bash-completion` is installed and does workk.

## Bash Completion

You can install it via your distribution's package manager.

For Ubuntu, `apt-get install bash-completion`

For CentOS, `yum install -y bash-completion`

## Setup Completion

Write bash completion code to a file and source if from .bash_profile.

```
mkdir -p ~/.config/kk/ && kk completion bash > ~/.config/kk/completion.bash.inc
printf "
# kk shell completion
source '$HOME/.config/kk/completion.bash.inc'
" >> $HOME/.bash_profile
source $HOME/.bash_profile
```
