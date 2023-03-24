add_library(timeout timeout.cpp)
add_fuzz_test(timeout_fuzztest timeout_fuzztest.cpp)
target_link_libraries(timeout_fuzztest PRIVATE timeout)
