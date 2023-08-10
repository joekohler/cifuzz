module.exports = {
	projects: [
		{
			displayName: "test",
		},
		{
			runner: "@jazzer.js/jest-runner",
			displayName: {
				name: "Jazzer.js",
				color: "cyan",
			},
			testMatch: ["<rootDir>/**/*.fuzz.js"],
		},
	],
};
