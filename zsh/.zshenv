# XDG
export XDG_CONFIG_HOME=$HOME/.config
export XDG_CACHE_HOME=$HOME/.cache
export XDG_DATA_HOME=$HOME/.local/share
export XDG_STATE_HOME=$HOME/.local/state

# locale
# ja_JP.UTF-8 is not in the system locale archive here and generating it needs
# root, so bootstrap compiles it into $XDG_DATA_HOME/locale with localedef and
# LOCPATH points glibc there. Without this every subprocess warns
# "Cannot set LC_CTYPE to default locale".
# Guarded because a LOCPATH aimed at a missing directory breaks *all* locales:
# glibc searches LOCPATH exclusively and stops consulting /usr/lib/locale.
# (C and C.UTF-8 are built into glibc, so they keep working either way.)
[[ -d $XDG_DATA_HOME/locale ]] && export LOCPATH=$XDG_DATA_HOME/locale
export LANGUAGE=en_US.UTF-8
export LANG=ja_JP.UTF-8

# editor
export EDITOR=nvim
export CVSEDITOR=$EDITOR
export SVN_EDITOR=$EDITOR
export GIT_EDITOR=$EDITOR

# history
export HISTFILE=${ZDOTDIR:-$XDG_CONFIG_HOME/zsh}/.zsh_history
export HISTSIZE=100000
export SAVEHIST=100000

# tool homes
export PNPM_HOME=$XDG_DATA_HOME/pnpm
export DOTNET_ROOT=$HOME/.dotnet
export NVM_DIR=$XDG_CONFIG_HOME/nvm

# PATH
# Declared here rather than in .zshrc so non-interactive shells (zsh -c,
# scripts, editor subshells) see the same toolchain. List whatever you might
# want; the (N-/) filter below keeps only the directories that actually exist,
# so uninstalled tools cost nothing and never need their own conditional.
typeset -U path
path=(
    $HOME/.local/bin
    $HOME/.cargo/bin
    $XDG_DATA_HOME/bob/nvim-bin
    $HOME/.pixi/bin
    $PNPM_HOME/bin              # pnpm >=10 puts `pnpm add -g` binaries here
    $PNPM_HOME                  # ...and its own launcher scripts here
    $HOME/.local/go/bin         # pinned Go (apt's patch version isn't ours to control)
    ${GOPATH:-$HOME/go}/bin
    $HOME/.nimble/bin
    $HOME/.elan/bin             # Lean
    $HOME/.sdkman/candidates/java/current/bin   # SDKMAN's java, without sourcing its init
    $XDG_DATA_HOME/zig
    $DOTNET_ROOT
    /usr/local/sbin
    $path
)

# WSL appends ~16 Windows directories to PATH. Building zsh's command hash
# means reading every one of them over drvfs, which costs ~80ms per shell --
# more than everything else here combined. Keep only the ones actually used;
# anything else remains reachable by its full path.
# To restore the lot, delete this block (or set appendWindowsPath in wsl.conf).
() {
    local -a keep=(
        '*/Microsoft VS Code/bin*'  # code
        '/mnt/[a-z]/Windows/[sS]ystem32'  # clip.exe, cmd.exe, where.exe
        '/mnt/[a-z]/Windows'        # explorer.exe
        '*/WinGet/Links*'           # winget-installed shims
        '*/WindowsApps*'            # winget.exe, store-installed apps
    )
    local -a kept=()
    local d p
    for d in ${(M)path:#/mnt/[a-z]/*}; do
        for p in $keep; do
            [[ $d == ${~p} ]] && { kept+=$d; break }
        done
    done
    path=( ${path:#/mnt/[a-z]/*} $kept )
}

path=( ${^path}(N-/) )

# Default node, without paying nvm.sh's ~620ms on every shell. nvm itself is
# lazy-loaded in .zshrc; this only puts the `default` alias on PATH.
() {
    local want
    [[ -r $NVM_DIR/alias/default ]] && want=$(<$NVM_DIR/alias/default)
    local -a m=( $NVM_DIR/versions/node/v${want#v}*/bin(N/n) )
    (( $#m )) || m=( $NVM_DIR/versions/node/*/bin(N/n) )
    (( $#m )) && path=( $m[-1] $path )
}
