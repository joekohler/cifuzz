package com.collection;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

public class NegativeArraySizeFuzzTest {
    void negative(int i) {
        String[] arr = new String[-i];
    }
    
    @FuzzTest
    void fuzzTest(FuzzedDataProvider data) {
        negative(data.consumeInt());
    }
}
