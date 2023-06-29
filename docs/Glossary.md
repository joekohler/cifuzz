# Glossary

To make sure we are talking about the same things we rely on the
following definitions:

## Fuzzer aka Fuzzing Engine

Is a piece of software that generates input to feed into the system
under test (SUT) via the fuzz tests. A coverage guided fuzzer will
also collect information about the covered code during the execution
and will try to improve coverage by adjusting the inputs accordingly.

## Fuzz Test

Usually a function that takes an array of bytes from the fuzzer and
calls functions from the system under test.

## Fuzzing

Fuzzing describes the act of running the fuzzer against a fuzz test.

## Corpus

A set of useful test inputs

### Seed Corpus

A small corpus usually provided by the user. Findings can be added to
this corpus for use in regression tests.
See https://llvm.org/docs/LibFuzzer.html#corpus.

### Generated Corpus

The generated corpus contains interesting inputs found by the fuzzer
while running the fuzz test.

## Replayer

A replayer runs inputs from the seed corpus against the fuzz test
without generating new input. It can be used for debugging and/or
regression testing.

## Sanitizer

Santizers use code instrumentation to detect bugs during the execution
of an application.

## Finding

A finding describes a bug or vulnerability found by the fuzzer and
includes the input that causes it.

## Metrics

While using **cifuzz** you will get in touch with metrics descriping
the progress or outcome of a fuzzing run. Some of them might have
different meanings depending on the context, previous knowledge or
might even be used differently by the various fuzzing engines.
To avoid confusion we provide the following definitions:

### exec/s

The average number of times per second the fuzz target has been called
with a generated input since fuzzing started.

### paths

While the fuzzer feeds generated inputs to a fuzz test, it progressively
explores the code under test.
The "paths" metric captures the different kinds of progress the fuzzer
can make, such as

- reaching new lines of code;
- executing a loop body a different number of times;
- satisfying equality for larger parts of a failing comparison.

While this number increasing can be taken as a sign that the fuzzer is
still making progress, it is not meaningful to compare across
different fuzz tests.

For the libFuzzer engine, the "paths" metric coincides with the
engine's "ft" (feature) count.

### last new path

The time that has passed since the last increase of the "paths" metric.

If this number keeps increasing, it is likely that the fuzzer isn't
making progress anymore. In this case, use `cifuzz coverage` to
get an idea of where the fuzzer got stuck.

## Coverage

Coverage describes the code reached (and therefore executed) during an
application/fuzzing run. It can be measured in different categories,
for example:

- Lines
- Functions
- Branches
