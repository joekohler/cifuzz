package com.collection;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

public class TimeoutFuzzTest {
    void infiniteLoop(String s) {
        if (s.startsWith("@")) {
            while (true){}
        }
    }

    @FuzzTest
    void fuzzTest(FuzzedDataProvider data) {
        infiniteLoop(data.consumeRemainingAsString());
    }
}
