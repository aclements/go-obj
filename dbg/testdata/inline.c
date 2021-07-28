void print(int x);

int x, y;

int funcC(int arg) {
    return arg + 1;
}

int funcB(int arg) {
    int a = funcC(arg * 2);
    print(a);
}

void funcA(void) {
    int a = funcB(x);
    print(a);
    int b = funcB(y);
    print(b);
}

int main(int argc, char **argv) {
    funcA();
}
