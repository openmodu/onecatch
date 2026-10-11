package keeldesktop

import (
	"github.com/dyike/keel/ui/locale"
	"testing"
)

func TestInterfacePreferencesPersistWithoutChangingUserContent(t *testing.T) {
	defer locale.Apply(locale.Chinese())
	a := conversationFixture(t)
	a.setLanguage("en")
	a.setChatFontSize("large")
	saved, err := loadUIPreferences(a.backend.service.DataRoot())
	if err != nil || saved.Language != "en" || saved.ChatFontSize != "large" {
		t.Fatalf("preferences: %+v, %v", saved, err)
	}
	if got := uiText("你想推进什么？"); got != "What would you like to work on?" {
		t.Fatal(got)
	}
	if got := uiText("用户自己的项目名"); got != "用户自己的项目名" {
		t.Fatal("unknown content translated:", got)
	}
	a.setLanguage("zh-CN")
	if got := uiText("设置"); got != "设置" {
		t.Fatal(got)
	}
}
