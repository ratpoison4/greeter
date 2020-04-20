package main

import (
	"bytes"
	"flag"
	"io/ioutil"
	"log"
	"time"

	tb "gopkg.in/tucnak/telebot.v2"
)

var (
	botTokenFile  = flag.String("telegram-bot-token", "token.txt", "File with bot token")
	greetTextFile = flag.String("greet-text", "greet.txt", "File with greeting")
	markdown      = flag.Bool("markdown", false, "If the file is formatted in markdown")
	delay         = flag.Duration("delay", 5*time.Minute, "Min delay between bot's messages")
)

func main() {
	flag.Parse()

	telegramToken, err := ioutil.ReadFile(*botTokenFile)
	if err != nil {
		panic(err)
	}

	greetText, err := ioutil.ReadFile(*greetTextFile)
	if err != nil {
		panic(err)
	}

	bot, err := tb.NewBot(tb.Settings{
		Token:  string(bytes.TrimSpace(telegramToken)),
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		panic(err)
	}

	send := func(recepient tb.Recipient) {
		var options []interface{}
		if *markdown {
			options = append(options, tb.ModeMarkdown)
		}
		options = append(options, tb.NoPreview)
		bot.Send(recepient, string(greetText), options...)
	}

	type State struct {
		sentLastTime        time.Time
		botsMessageIsLatest bool
	}

	chat2state := make(map[int64]*State)

	bot.Handle("/start", func(m *tb.Message) {
		if !m.Private() {
			return
		}
		send(m.Sender)
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

		send(m.Chat)

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

	bot.Start()
}
