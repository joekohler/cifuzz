Output docs/assets/gradle_showcase.gif

Require cifuzz
Require gradle
Require nvim

Set FontSize 18
Set Width 1200
Set Height 600

Sleep 100ms

Hide
Type "cd examples/gradle"
Enter
Show

Sleep 1s 

Type "cifuzz init"
Enter
Sleep 2s

Type "nvim build.gradle"
Enter
Sleep 1s

Type "20jo"
Type@20ms "testImplementation 'com.code-intelligence:jazzer-junit:0.16.0'"
Escape
Sleep 3
Type ":wq"
Enter

Type@30ms "cifuzz create -o src/test/java/com/example/MyFuzzTest.java"
Enter
Sleep 1s
Down
Sleep 500ms
Enter
Sleep 2s

Type@30ms "nvim src/test/java/com/example/MyFuzzTest.java"
Enter
Sleep 500ms

Type "8j"
Sleep 1s
Type "ci}"
Type@20ms "int a = data.consumeInt();"
Enter
Type@20ms "int b = data.consumeInt();"
Enter
Type@20ms "String c = data.consumeRemainingAsString();"
Enter
Enter
Type@20ms "ExploreMe ex = new ExploreMe(a);"
Enter
Type@20ms "ex.exploreMe(b, c);"

Escape
Sleep 3s

Type ":wq"
Enter

Type "cifuzz run com.example.MyFuzzTest"
Enter

Sleep 6s 

Type "cifuzz findings"
Enter 

Sleep 3s 
