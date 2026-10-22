package service

import (
	"strings"
	"testing"
)

// TestOperatorCredentialMessages locks the wording operators actually see.
// "Неверный email или пароль" made no sense (the panel logs in with a handle,
// not an email) and the /bind answer has to show the expected login format.
func TestOperatorCredentialMessages(t *testing.T) {
	for _, message := range []string{WrongCredentials, BindWrongCredentials} {
		if strings.Contains(strings.ToLower(message), "email") {
			t.Errorf("message still mentions email: %q", message)
		}
		if !strings.Contains(strings.ToLower(message), "пароль") {
			t.Errorf("message should mention the password: %q", message)
		}
	}

	if !strings.Contains(BindWrongCredentials, "@operator_almaty") {
		t.Errorf("the /bind answer should show the expected handle: %q", BindWrongCredentials)
	}
	if WrongCredentials == BindWrongCredentials {
		t.Error("the /bind answer should be more specific than the login error")
	}
}
