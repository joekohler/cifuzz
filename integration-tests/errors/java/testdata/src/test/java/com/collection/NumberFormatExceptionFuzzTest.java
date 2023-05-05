package com.collection;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

public class NumberFormatExceptionFuzzTest {
    void numberFormat(String s) {
        if (s.startsWith("@")) {
            Integer.parseInt(null);
        }
    }

    @FuzzTest
    void fuzzTest(FuzzedDataProvider data) {
        numberFormat(data.consumeRemainingAsString());
    }
    
}
