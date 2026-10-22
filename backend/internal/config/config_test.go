package config

import "testing"

func TestMiniAppLink(t *testing.T) {
	withApp := &Config{}
	withApp.Bot.Username = "srunal_bot"
	withApp.Bot.MiniAppShortName = "srunal"

	cases := []struct {
		name       string
		config     *Config
		startParam string
		want       string
	}{
		{
			name:       "order screen",
			config:     withApp,
			startParam: "order_DG-260391",
			want:       "https://t.me/srunal_bot/srunal?startapp=order_DG-260391",
		},
		{
			name:       "no parameter",
			config:     withApp,
			startParam: "",
			want:       "https://t.me/srunal_bot/srunal",
		},
		{
			name:       "parameter is escaped",
			config:     withApp,
			startParam: "review DG/1",
			want:       "https://t.me/srunal_bot/srunal?startapp=review+DG%2F1",
		},
		{
			name:       "without a short name there is no link",
			config:     &Config{},
			startParam: "order_DG-1",
			want:       "",
		},
	}

	for _, testCase := range cases {
		if testCase.name == "without a short name there is no link" {
			testCase.config.Bot.Username = "srunal_bot" // handle set, short name missing
		}
		if got := testCase.config.MiniAppLink(testCase.startParam); got != testCase.want {
			t.Fatalf("%s: MiniAppLink(%q) = %q, want %q", testCase.name, testCase.startParam, got, testCase.want)
		}
	}
}
