package com.collection;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

public class ArrayOutOfBoundsFuzzTest {
    void outOfBounds(int i) {
        String[] arr = new String[i];
        System.out.println(arr[i]);
    }
    
    @FuzzTest
    void fuzzTest(FuzzedDataProvider data) {
        outOfBounds(data.consumeInt());
    }
}
