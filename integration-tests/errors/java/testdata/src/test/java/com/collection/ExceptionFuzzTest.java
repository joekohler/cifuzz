package com.collection;

import com.code_intelligence.jazzer.api.FuzzedDataProvider;
import com.code_intelligence.jazzer.junit.FuzzTest;
import java.lang.SecurityException;
import java.lang.Exception;

public class ExceptionFuzzTest {
		@FuzzTest
		void fuzzTestException(FuzzedDataProvider data) throws Exception {
				throw new Exception("Exception");
		}

		@FuzzTest
		void fuzzTestSecurityException(FuzzedDataProvider data) throws SecurityException {
				throw new SecurityException("SecurityException");
		}
}
