#include "slow_input.cpp"

#include <cifuzz/cifuzz.h>

FUZZ_TEST_SETUP() {
}

FUZZ_TEST(const uint8_t *data, size_t size) {
    slow(size);
}
