# cifuzz CMake reference

The `cifuzz` CMake integration provides a set of CMake functions to simplify the process of writing fuzz tests.

When `cifuzz` is installed, the integration can be loaded by adding the following line to your top-level `CMakeLists.txt`:

```cmake
find_package(cifuzz NO_SYSTEM_ENVIRONMENT_PATH)
```

## Initial Setup

```cmake
enable_fuzz_testing()
```

Call this function in your top-level `CMakeLists.txt` to enable support for fuzz testing.

This function does not modify any settings unless the `CIFUZZ_TESTING` cache variable is set to `ON`.
`cifuzz` sets this variable when it builds the project for fuzzing.

To manually enable sanitizer instrumentation, for example when running regression tests, set `CIFUZZ_TESTING` to `ON` and add one or more sanitizer names to `CIFUZZ_SANIIZERS` (supported: `address`, `undefined`).
Also see the dedicated documentation for [Regression Testing](../Regression-Testing.md).

## Defining a fuzz test

```cmake
add_fuzz_test(<name> [source1] [source2 ...])
```

Use this function to define a fuzz test, which consists of a fuzz test executable `<name>` as well as a corresponding regression test target `<name>_regression_test`.

To add libraries to the fuzz test or modify any other settings, use the `target_*(<name> ...)` functions as usual.

### Defining a Fuzz Test and its Dependencies (Legacy)

```cmake
add_fuzz_test(<name>
              SOURCES [source1] [source2 ...]
             [DEPENDENCIES [library1] [library2 ...]]
             [INCLUDE_DIRS [include1] [include2 ...]])
```

A shorthand for defining a fuzz test and adding libraries and include directories to it.

Equivalent to:

```cmake
add_fuzz_test(<name> [source1] [source2 ...])
target_link_libraries(<name> [library1] [library2 ...])
target_include_directories(<name> [include1] [include2 ...])
```

Prefer the form without keyword arguments for consistency with other CMake functions.
There are no concrete plans to remove the keyword argument form.
