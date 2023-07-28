#![no_main]
#[macro_use] extern crate libfuzzer_sys;

fuzz_target!(|data: &[u8]| {
    let v = vec![1, 2, 3];

    v[3];
});