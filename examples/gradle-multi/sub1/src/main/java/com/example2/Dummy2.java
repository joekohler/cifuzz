package com.example2;

public class Dummy2 {
	private int a;

	public Dummy2(int a) {
		this.a = a;
	}

	public void foo(String c) {
		if (this.a >= 1000) {
			System.out.println(c);
		}
	}
}
