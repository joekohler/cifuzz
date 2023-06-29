# Troubleshooting

## Cifuzz headers not found in CMake project

If you encounter the following error message in your CMake project:

```
fatal error: 'cifuzz/cifuzz.h' file not found
#include <cifuzz/cifuzz.h>
         ^~~~~~~~~~~~~~~~~
```

You can try including the cifuzz directory explicitly to the fuzz test
declaration in your `CMakeLists.txt` with a `target_include_directories` directive.

```
add_fuzz_test(my_fuzz_test my_fuzz_test.cpp)
target_link_libraries(my_fuzz_test PRIVATE exploreMe)
target_include_directories(my_fuzz_test PUBLIC $ENV{HOME}/.local/share/cifuzz/include)
```
