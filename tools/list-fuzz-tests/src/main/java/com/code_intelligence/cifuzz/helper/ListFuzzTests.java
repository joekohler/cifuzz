package com.code_intelligence.cifuzz.helper;

import static java.util.Arrays.asList;

import java.util.List;
import java.util.logging.ConsoleHandler;
import java.util.logging.Level;
import java.util.logging.Logger;

/**
 * Scans the classpath for fuzz tests and emits one line per test with its identifier in the form
 * {@code com.example.MyFuzzTest} or {@code com.example.MyTests::fuzzTest}.
 *
 * <p>Execute via: {@code java -cp path/to/list-fuzz-tests.jar:...
 * com.code_intelligence.cifuzz.helper.ListFuzzTests <class name>...}
 *
 * <p>If no class names are provided, all directories (but not JAR files) on the classpath are
 * scanned for tests. If one or more class name is provided, only these classes are scanned.
 *
 * <p>The tool first checks whether JUnit 5 is available on the classpath and looks for
 * {@link com.code_intelligence.jazzer.junit.FuzzTest}s. If it is not available, it instead looks
 * for public classes with a public static {code fuzzerTestOneInput} function.
 */
public class ListFuzzTests {

  public static void main(String[] args) {
    // JUnit does not report errors if class files could not be loaded successfully, rather it just
    // logs an appropriate message and continues.
    if (System.getenv("CIFUZZ_VERBOSE") != null) {
      Logger logger = Logger.getLogger("org.junit");
      logger.setLevel(Level.FINE);
      ConsoleHandler consoleHandler = new ConsoleHandler();
      consoleHandler.setLevel(Level.FINE);
      logger.addHandler(consoleHandler);
    }

    List<String> classes = asList(args);
    JUnitFuzzTestLister.listFuzzTests(classes)
        .orElseGet(() -> LegacyFuzzTestLister.listFuzzTests(classes))
        // Emit consistent line endings across platforms.
        .forEach(fuzzTest -> System.out.print(fuzzTest + "\n"));
  }
}
