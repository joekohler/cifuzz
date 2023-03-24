#include <string>
using namespace std;

void out_of_bounds(string c) {
    if (c == "FUZZING") {
        int arr[] = {34,56,66,78};
        printf("arr[12] is %d\n", arr[12]);
    }
}
