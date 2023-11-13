# cifuzz configuration

You can change the behavior of **cifuzz** both via command-line flags
and via settings stored in the `cifuzz.yaml` config file. Flags take
precedence over the respective config file setting.

## cifuzz.yaml settings

[build-system](#build-system) <br/>
[build-command](#build-command) <br/>
[seed-corpus-dirs](#seed-corpus-dirs) <br/>
[dict](#dict) <br/>
[engine-args](#engine-args) <br/>
[timeout](#timeout) <br/>
[use-sandbox](#use-sandbox) <br/>
[print-json](#print-json) <br/>
[no-notifications](#no-notifications) <br/>
[server](#server) <br/>
[project](#project) <br/>
[style](#style) <br/>

<a id="build-system"></a>

### build-system

The build system used to build this project. If not set, cifuzz tries
to detect the build system automatically.
Valid values: "bazel", "cmake", "maven", "gradle", "other".

#### Example

```yaml
build-system: cmake
```

<a id="build-command"></a>

### build-command

If the build system type is "other", this command is used by
`cifuzz run` to build the fuzz test.

#### Example

```yaml
build-command: "make all"
```

<a id="seed-corpus-dirs"></a>

### seed-corpus-dirs

Directories containing sample inputs for the code under test.
See https://llvm.org/docs/LibFuzzer.html#corpus.

#### Example

```yaml
seed-corpus-dirs:
  - path/to/seed-corpus
```

<a id="dict"></a>

### dict

A file containing input language keywords or other interesting byte
sequences.
See https://llvm.org/docs/LibFuzzer.html#dictionaries.

#### Example

```yaml
dict: path/to/dictionary.dct
```

<a id="engine-args"></a>

### engine-args

Command-line arguments to pass to libFuzzer or Jazzer for running fuzz tests.
Engine-args are not supported for running `cifuzz coverage` on JVM-projects
and are not supported for Node.js projects.

For possible libFuzzer options see https://llvm.org/docs/LibFuzzer.html#options.

For advanced configuration with Jazzer parameters see https://github.com/CodeIntelligenceTesting/jazzer/blob/main/docs/advanced.md.

Fuzzer customization for Node.js projects can be specified in `.jazzerjsrc.json`
in the root project directory. See https://github.com/CodeIntelligenceTesting/jazzer.js/blob/main/docs/jest-integration.md
for further information.

#### Example Libfuzzer

```yaml
engine-args:
  - -rss_limit_mb=4096
  - -timeout=5s
```

#### Example Jazzer

```yaml
engine-args:
  - --instrumentation_includes=com.**
  - --keep_going
```

<a id="timeout"></a>

### timeout

Maximum time in seconds to run the fuzz tests. The default is to run
indefinitely.

#### Example

```yaml
timeout: 300
```

<a id="use-sandbox"></a>

### use-sandbox

By default, fuzz tests are executed in a sandbox to prevent accidental
damage to the system. Set to false to run fuzz tests unsandboxed.
Only supported on Linux.

#### Example

```yaml
use-sandbox: false
```

<a id="print-json"></a>

### print-json

Set to true to print output of the `cifuzz run` command as JSON.

#### Example

```yaml
print-json: true
```

### no-notifications

Set to true to disable desktop notifications

#### Example

```yaml
no-notifications: true
```

### server

Set URL of CI Sense

#### Example

```yaml
server: https://app.code-intelligence.com
```

### project

Set the project name of CI Sense project

#### Example

```yaml
project: my-project-1a2b3c4d
```

### style

Choose the style to run cifuzz in

- `pretty`: Colored output and icons (default)
- `color`: Colored output
- `plain`: Pure text without any styles

#### Example

```yaml
style: plain
```

## Configuration of seed corpus and dictionary inputs

Seed corpus directories and a dictionary file can be defined for the whole project using the `cifuzz.yaml` config file.

Those settings can be overridden per fuzz test using the `--seed-corpus` and `--dict` flags.

### Default seed corpus directories

Each fuzz test has a default seed corpus location, whose inputs are used in addition to inputs defined in the `cifuzz.yaml` config file or
using the `--seed-corpus` flag. The reproducing inputs for findings are stored in this location.
This directory can also be used for custom inputs.

#### C/C++

```
<fuzz test name>_inputs
```

#### Java/Kotlin

```
src/test/resources/.../<fuzz test name>Inputs
```

#### Javascript/Typescript

```
<fuzz test name>.fuzz
```

### Defining dictionaries per fuzz test

Note that only one dictionary file can be defined per fuzz test.

#### C/C++

Similarly to default seed corpus directories, each fuzz test has a default dictionary location.
This dictionary is used if no other dictionary is defined in the `cifuzz.yaml` config file or
using the `dict` flag.

```
<fuzz test name>.dict
```

#### Java/Kotlin

Dictionary entries can be defined with the help of JUnit annotations, e.g.:

```java
@DictionaryFile(resourcePath = "test.dict")
@DictionaryEntries("test")
```

Note that it is possible to define multiple files or entries per fuzz test.

#### Javascript/Typescript

Support coming soon.
