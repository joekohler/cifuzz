package com.example;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertTrue;

public class TestCases{
		@FuzzTest
		void myFuzzTest1(FuzzedDataProvider data) {
				int a = data.consumeInt();
				int b = data.consumeInt();
				String c = data.consumeRemainingAsString();
				throw new NullPointerException();

		}

    @FuzzTest
    void myFuzzTest2(FuzzedDataProvider data) {
        int a = data.consumeInt();
        int b = data.consumeInt();
        String c = data.consumeRemainingAsString();
				throw new RuntimeException();
    }

    @Test
    public void unitTest() {
        ExploreMe ex = new ExploreMe(100);
        ex.exploreMe(100, "Test");
    }
}
