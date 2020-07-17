package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ratpoison4/entities"
	tb "gopkg.in/tucnak/telebot.v2"
)

var (
	botTokenFile = flag.String("telegram-bot-token", "token.txt", "File with bot token")
	greetDir     = flag.String("greet-dir", "", "Directory with greetings")
	delay        = flag.Duration("delay", 5*time.Minute, "Min delay between bot's messages")
)

func main() {
	flag.Parse()

	telegramToken, err := ioutil.ReadFile(*botTokenFile)
	if err != nil {
		panic(err)
	}

	type State struct {
		text                string
		sentLastTime        time.Time
		prev                *tb.Message
		botsMessageIsLatest bool
	}

	chat2state := make(map[int64]*State)

	infos, err := ioutil.ReadDir(*greetDir)
	if err != nil {
		panic(err)
	}
	defaultText := "Hello"
	for _, info := range infos {
		name := info.Name()
		path := filepath.Join(*greetDir, name)
		textBytes, err := ioutil.ReadFile(path)
		if err != nil {
			panic(err)
		}
		text := string(textBytes)
		if name == "default.md" {
			defaultText = text
			continue
		}
		chatID, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSuffix(name, ".md"), "chat"))
		if err != nil {
			log.Printf("Can not extract chat ID from file name %s: %v.", name, err)
			continue
		}
		chat2state[int64(chatID)] = &State{text: text}
	}

	bot, err := tb.NewBot(tb.Settings{
		Token:  string(bytes.TrimSpace(telegramToken)),
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		panic(err)
	}

	send := func(recepient tb.Recipient, text string) (*tb.Message, error) {
		return bot.Send(recepient, text, tb.ModeMarkdownV2, tb.NoPreview)
	}

	bot.Handle("/start", func(m *tb.Message) {
		log.Printf("/start")
		if !m.Private() {
			return
		}
		_, err := send(m.Sender, defaultText)
		if err != nil {
			log.Printf("Replying to /start failed: %v.", err)
		}
	})

	bot.Handle("/use", func(m *tb.Message) {
		if m.Chat == nil {
			return
		}

		admins, err := bot.AdminsOf(m.Chat)
		if err != nil {
			bot.Reply(m, "Can not get the list of chat admins.")
			log.Printf("Chat %q. Can not get the list of chat admins: %v.", m.Chat.Title, err)
			return
		}
		isAdmin := false
		for _, admin := range admins {
			if admin.User.ID == m.Sender.ID {
				isAdmin = true
			}
		}
		if !isAdmin {
			bot.Reply(m, "You are not admin.")
			return
		}

		if m.ReplyTo == nil {
			bot.Reply(m, "Use this command in reply to the message you want to make the greeting.")
			return
		}

		text := entities.ConvertToMarkdownV2(m.ReplyTo.Text, m.ReplyTo.Entities)

		state, has := chat2state[m.Chat.ID]
		if !has {
			state = &State{}
			chat2state[m.Chat.ID] = state
		}
		state.text = text

		path := filepath.Join(*greetDir, fmt.Sprintf("chat%d.md", m.Chat.ID))
		if err := ioutil.WriteFile(path, []byte(text), 0644); err != nil {
			log.Printf("Chat %q. Failed to save text to file %s: %v.", m.Chat.Title, path, err)
		}

		state.botsMessageIsLatest = false

		bot.Reply(m, "OK")
	})

	bot.Handle(tb.OnUserJoined, func(m *tb.Message) {
		if m.Chat == nil {
			return
		}

		log.Printf("Chat %q. User joined.", m.Chat.Title)

		state, has := chat2state[m.Chat.ID]
		if !has {
			state = &State{}
			chat2state[m.Chat.ID] = state
		}

		if state.botsMessageIsLatest {
			log.Printf("Chat %q. Not posting because there is already bot's message in the end of the chat.", m.Chat.Title)
			return
		}
		passed := time.Since(state.sentLastTime)
		if passed < *delay {
			log.Printf("Chat %q. Not posting because only %s passed from bot's latest message, required minimum delay is %s.", m.Chat.Title, passed, delay)
			return
		}

		if state.prev != nil {
			bot.Delete(state.prev)
		}

		text := defaultText
		if state.text != "" {
			text = state.text
		}

		state.prev, err = send(m.Chat, text)
		if err != nil {
			log.Printf("Chat %q. Failed to send: %v.", m.Chat.Title, err)
		}

		state.sentLastTime = time.Now()
		state.botsMessageIsLatest = true
	})

	reset := func(m *tb.Message) {
		if m.Chat == nil {
			return
		}

		log.Printf("Chat %q. Reset of botsMessageIsLatest.", m.Chat.Title)

		state, has := chat2state[m.Chat.ID]
		if !has {
			state = &State{}
			chat2state[m.Chat.ID] = state
		}

		state.botsMessageIsLatest = false
	}

	bot.Handle(tb.OnText, reset)
	bot.Handle(tb.OnPhoto, reset)
	bot.Handle(tb.OnAudio, reset)
	bot.Handle(tb.OnSticker, reset)
	bot.Handle(tb.OnVoice, reset)

	log.Printf("Starting bot.")
	bot.Start()
}
