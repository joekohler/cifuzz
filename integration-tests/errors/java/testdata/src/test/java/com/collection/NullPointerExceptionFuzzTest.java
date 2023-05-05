package com.collection;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

public class NullPointerExceptionFuzzTest {
    void nullPointer(String s) {
        String ptr = null;
        if (ptr.equals(s)) {
            System.out.println("what are you trying to do?");
        }
    }
    
    @FuzzTest
    void fuzzTest(FuzzedDataProvider data) {
        nullPointer(data.consumeRemainingAsString());
    }
}
