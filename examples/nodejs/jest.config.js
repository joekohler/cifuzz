module.exports = {
	collectCoverageFrom: ["src/**/*.{js,jsx}"],
	projects: [
		{
			displayName: "test",
		},
		{
			testRunner: "@jazzer.js/jest-runner",
			displayName: {
				name: "Jazzer.js",
				color: "cyan",
			},
			testMatch: ["<rootDir>/**/*.fuzz.js"],
		},
	],
};
