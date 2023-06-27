
public class FuzzTestCase {
    @FuzzTest
    void oneFuzzTest(FuzzedDataProvider data) {}

    @FuzzTest
    void anotherFuzzTest(FuzzedDataProvider data) {}
}
