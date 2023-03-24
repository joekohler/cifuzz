#include <string>
using namespace std;

int *gp;
bool b = true;

void use_after_scope(string c) {
    if (c == "FUZZING") {
        if (b) {
            int x[5];
            gp = x+1;
        }
        *gp = 5;
    }
}
