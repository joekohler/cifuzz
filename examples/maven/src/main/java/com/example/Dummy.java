package com.example;

public class Dummy {
	private int a;

	public Dummy(int a) {
		this.a = a;
	}

	public void foo(String c) {
		if (this.a >= 1000) {
			System.out.println(c);
		}
	}
}
