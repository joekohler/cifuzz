module code-intelligence.com/cifuzz

go 1.19

require (
	atomicgo.dev/keyboard v0.2.9
	github.com/Masterminds/semver v1.5.0
	github.com/alessio/shellescape v1.4.1
	github.com/alexflint/go-filemutex v1.2.0
	github.com/gen2brain/beeep v0.0.0-20220909211152-5a9ec94374f6
	github.com/gookit/color v1.5.2
	github.com/hectane/go-acl v0.0.0-20190604041725-da78bae5fc95
	github.com/hokaccha/go-prettyjson v0.0.0-20211117102719-0474bc63780f
	github.com/mattn/go-zglob v0.0.4
	github.com/mitchellh/ioprogress v0.0.0-20180201004757-6a23b12fa88e
	github.com/otiai10/copy v1.9.0
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8
	github.com/pkg/errors v0.9.1
	github.com/pterm/pterm v0.12.51-0.20221221034244-22f4f9645a9f
	github.com/spf13/cobra v1.6.1
	github.com/spf13/viper v1.15.0
	github.com/u-root/u-root v0.11.0
	golang.org/x/net v0.5.0
	golang.org/x/sync v0.1.0
	golang.org/x/term v0.4.0
)

// TODO: Revert when https://github.com/otiai10/copy/pull/94 is merged
replace github.com/otiai10/copy v1.9.0 => github.com/adombeck/copy v0.0.0-20221129184714-7b5f9872f143

require (
	atomicgo.dev/cursor v0.1.1 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-toast/toast v0.0.0-20190211030409-01e6764cf0a4 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/lithammer/fuzzysearch v1.1.5 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/spf13/afero v1.9.3 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/tadvi/systray v0.0.0-20190226123456-11a2b8fa57af // indirect
	github.com/xo/terminfo v0.0.0-20210125001918-ca9a967f8778 // indirect
)

require (
	github.com/fatih/color v1.13.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/pelletier/go-toml/v2 v2.0.6 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.8.1
	github.com/subosito/gotenv v1.4.2 // indirect
	golang.org/x/exp v0.0.0-20220414153411-bcd21879b8fd
	golang.org/x/sys v0.4.0
	golang.org/x/text v0.6.0
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1
)
