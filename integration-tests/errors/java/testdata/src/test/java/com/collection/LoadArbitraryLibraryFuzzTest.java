package com.collection;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

public class LoadArbitraryLibraryFuzzTest {
    void loadLibrary(String input) {
        try {
            System.loadLibrary(input);
        } catch (SecurityException | UnsatisfiedLinkError | IllegalArgumentException ignored) {
        }
    }

    @FuzzTest
    void fuzzTest(FuzzedDataProvider data) {
        loadLibrary(data.consumeRemainingAsAsciiString());
    }
}
