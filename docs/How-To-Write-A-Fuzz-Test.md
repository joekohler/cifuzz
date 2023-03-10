# How to write a fuzz test

## Setup

### CMake
When using `cifuzz init` and `cifuzz create` the commands will tell you
which manual steps are necessary to use the cifuzz CMake integration inside
your existing project. Usually you also have to add instructions in your
CMakeLists.txt file to link the fuzz test with the software under test
(e.g. use the `target_link_libraries directive`).

The `add_fuzz_test` directive can be treated just like `add_executable`:

```
add_fuzz_test(my_fuzz_test my_fuzz_test.cpp)

target_link_libraries(my_fuzz_test my_library)
target_compile_definitions(my_fuzz_test PRIVATE MY_DEFINE=foo)
```

More detailed information can be found in the [CMake reference](../cmake/Reference.md).

## How to convert/cast the fuzzer data into the data types you need

You might have to convert/cast the input parameters to other types to call your
functions. A useful tool for this is The
[FuzzedDataProvider](https://github.com/google/fuzzing/blob/master/docs/split-inputs.md#fuzzed-data-provider).

<details>
<summary>C/C++</summary>

An example can look like this:

``` cpp
#include <cifuzz/cifuzz.h>
#include <fuzzer/FuzzedDataProvider.h>

FUZZ_TEST_SETUP() {}

FUZZ_TEST(const uint8_t *data, size_t size) {

  FuzzedDataProvider fuzzed_data(data, size);
  int my_int = fuzzed_data.ConsumeIntegral<int8_t>();
  std::string my_string = fuzzed_data.ConsumeRandomLengthString();

  myFunction(my_int, my_string);
}
```
</details>

<details>
<summary>Java</summary>
For Java, you can use the FuzzedDataProvider which is part of the Jazzer API
package that is automatically downloaded by maven/gradle respectively if set up
properly after cifuzz init.

An example can look like this:

```java
import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

public class FuzzTestCase {
    @FuzzTest
    void myFuzzTest(FuzzedDataProvider data) {
        int a = data.consumeInt();
        int b = data.consumeInt();
        String c = data.consumeRemainingAsString();

        myFunction(a, b, c);
    }
}
```
</details>


