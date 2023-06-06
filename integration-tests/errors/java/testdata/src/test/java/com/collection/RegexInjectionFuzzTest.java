package com.collection;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;

import java.util.regex.Pattern;
import java.util.regex.PatternSyntaxException;

public class RegexInjectionFuzzTest {
    void canonEQ(String input) {
        try {
            Pattern.compile(Pattern.quote(input), Pattern.CANON_EQ);
        } catch (PatternSyntaxException ignored) {
        } catch (IllegalArgumentException ignored) {
            // "[媼" generates an IllegalArgumentException but only on Windows using Java 8. 
            // We ignore this for now.
        }
    }
    
    void insecureQuote(String input) {
        try {
            Pattern.matches("\\Q" + input + "\\E", "foobar");
        } catch (PatternSyntaxException ignored) {
        }
    }

    @FuzzTest
    void fuzzTestInsecureQuote(FuzzedDataProvider data) {
        insecureQuote(data.consumeRemainingAsString());
    }

    @FuzzTest
    void fuzzTestICanonEQ(FuzzedDataProvider data) {
        canonEQ(data.consumeRemainingAsString());
    }
}
