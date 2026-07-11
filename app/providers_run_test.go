package main

import (
	"testing"

	"vk-parser/internal/config"
)

func TestSelectProvider(t *testing.T) {
	withToken := &config.Config{VKToken: "t", DataDir: "data"}

	t.Run("glamping не требует токена", func(t *testing.T) {
		p, err := selectProvider("glamping", &config.Config{})
		if err != nil || p.Name() != "glamping_rf" {
			t.Fatalf("glamping: p=%v err=%v", p, err)
		}
	})

	t.Run("vk с токеном — ок", func(t *testing.T) {
		p, err := selectProvider("vk", withToken)
		if err != nil || p.Name() != "vk" {
			t.Fatalf("vk: p=%v err=%v", p, err)
		}
	})

	// Fail-fast: без VK_TOKEN провайдер vk обязан падать на старте, а не
	// «успешно» собирать пустой objects.json (каждый домен молча падал бы).
	t.Run("vk без токена — ошибка на старте", func(t *testing.T) {
		if _, err := selectProvider("vk", &config.Config{}); err == nil {
			t.Fatal("ожидал ошибку про VK_TOKEN, получил nil")
		}
	})

	t.Run("неизвестное имя — ошибка", func(t *testing.T) {
		if _, err := selectProvider("nope", withToken); err == nil {
			t.Fatal("ожидал ошибку про неизвестный провайдер")
		}
	})
}
