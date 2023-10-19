# Demo project for fuzzing clojure code

Example clojure project with two Jazzer fuzz targets.

## Usage

Build a fuzzing JAR with

```shell
clojure "-T:build" "fuzzing-jar"
```

Then run the fuzzer as follows:

```shell
java -cp target/fuzzing.jar  com.code_intelligence.jazzer.Jazzer              \
     --target_class=jazzer_clojure_example.targets.SimpleExample
```

or

```shell
java -cp target/fuzzing.jar  com.code_intelligence.jazzer.Jazzer              \
       --target-class=jazzer_clojure_example.targets.JsonistaExample          \
       /fuzzing/corpus-jsonista
```
