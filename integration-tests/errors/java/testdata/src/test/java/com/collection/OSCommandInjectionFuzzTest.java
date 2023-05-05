package com.collection;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

import static java.lang.Runtime.getRuntime;

import java.util.concurrent.TimeUnit;

public class OSCommandInjectionFuzzTest {
    void commandInjection(String input) {
        try {
            Process process = getRuntime().exec(input, new String[] {});
            // This should be way faster, but we have to wait until the call is done
            if (!process.waitFor(10, TimeUnit.MILLISECONDS)) {
                process.destroyForcibly();
            }
        } catch (Exception ignored) {
            // Ignore execution and setup exceptions
        }
    }

    @FuzzTest
    void fuzzTest(FuzzedDataProvider data) {
        commandInjection(data.consumeRemainingAsAsciiString());
    }
}
