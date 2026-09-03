umask 022
limit coredumpsize 0

# Is a command available? Uses zsh's own command hash, so unlike
# `command -v` in $(...) it costs no fork. Guarding every optional tool with
# this keeps the config working on a machine where it isn't installed.
has() { (( $+commands[$1] )) }

# options
setopt EXTENDED_HISTORY share_history
setopt hist_ignore_dups hist_ignore_all_dups hist_ignore_space hist_verify
setopt hist_reduce_blanks hist_save_no_dups hist_no_store hist_expand
setopt list_packed no_beep
unsetopt bg_nice list_types
export LISTMAX=50

# PATH lives in .zshenv so scripts and non-interactive shells share it.

# plugins
# Must precede compinit: zsh-completions adds itself to fpath here.
eval "$(sheldon source)"

# completion
autoload -Uz compinit
() {
    local zcd=$XDG_CACHE_HOME/zsh/zcompdump
    [[ -d $zcd:h ]] || mkdir -p $zcd:h
    # Full security scan (compaudit, ~60ms) at most once a day; -C skips it.
    if [[ -n $zcd(#qN.mh-24) ]]; then
        compinit -C -d $zcd
    else
        compinit -d $zcd
    fi
    # Byte-compile the dump whenever it is newer than its .zwc; zsh then loads
    # the .zwc automatically. Backgrounded so it never costs startup time, and
    # the old .zwc is removed first because zcompile writes it read-only.
    if [[ -s $zcd && ( ! -s $zcd.zwc || $zcd -nt $zcd.zwc ) ]]; then
        { rm -f $zcd.zwc && zcompile -R -- $zcd.zwc $zcd } &!
    fi
}

# nvm
# nvm.sh is 4700 lines and runs `nvm use default` on load (~620ms). The
# default version is already on PATH via .zshenv, so pay that cost only if a
# real nvm command is issued.
if [[ -s $NVM_DIR/nvm.sh ]]; then
    nvm() {
        unfunction nvm
        source $NVM_DIR/nvm.sh
        [[ -s $NVM_DIR/bash_completion ]] && source $NVM_DIR/bash_completion
        nvm "$@"
    }
fi

# prompt
if has starship; then
    eval "$(starship init zsh)"
else
    PROMPT='%F{cyan}%~%f %# '
fi

# fzf
has fzf && source <(fzf --zsh)

# zoxide
has zoxide && eval "$(zoxide init zsh)"

# gpg
# pinentry has to be told which terminal to draw its passphrase prompt on;
# without this, signing a commit dies with "gpg: signing failed: Inappropriate
# ioctl for device". $TTY is zsh's own parameter, so unlike $(tty) it is free.
# Set here rather than .zshenv because a shell with no terminal has nothing to
# point pinentry at anyway.
[[ -n $TTY ]] && export GPG_TTY=$TTY

# aliases
source ${ZDOTDIR:-$XDG_CONFIG_HOME/zsh}/aliases.zsh

# functions
# An `if` rather than `&&` so a missing tool doesn't leave $? non-zero for the
# first prompt -- starship renders that as a command failure.
if has yazi; then
    function y() {
        local tmp="$(mktemp -t "yazi-cwd.XXXXXX")" cwd
        yazi "$@" --cwd-file="$tmp"
        if cwd="$(command cat -- "$tmp")" && [ -n "$cwd" ] && [ "$cwd" != "$PWD" ]; then
            builtin cd -- "$cwd"
        fi
        rm -f -- "$tmp"
    }
fi

#THIS MUST BE AT THE END OF THE FILE FOR SDKMAN TO WORK!!!
export SDKMAN_DIR="$HOME/.sdkman"
[[ -s "$HOME/.sdkman/bin/sdkman-init.sh" ]] && source "$HOME/.sdkman/bin/sdkman-init.sh"
