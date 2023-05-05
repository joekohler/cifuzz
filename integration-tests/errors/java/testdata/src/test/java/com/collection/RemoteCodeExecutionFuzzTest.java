package com.collection;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

class RemoteCodeExecutionFuzzTest {
    void remote(String s) {
        if (s.startsWith("@")) {
            try {
                Class.forName(s.substring(1)).getDeclaredConstructor().newInstance();
            } catch (Exception e) {
            }
        }
    }    
    
    @FuzzTest
    void fuzzTest(FuzzedDataProvider data) {
        remote(data.consumeRemainingAsString());
    }
}
