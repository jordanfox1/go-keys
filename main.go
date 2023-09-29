package main

import (
	"fmt"
	"keyboard/keys"

	"github.com/eiannone/keyboard"
)

func main() {
	c, op, err := keys.InitAudioContext()
	if err != nil {
		fmt.Println(err)
		return
	}

	keysEvents, err := keyboard.GetKeys(200)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() {
		_ = keyboard.Close()
	}()

	fmt.Println("Press ESC to quit")
	for {
		select {
		case event := <-keysEvents:
			keys.NoteCount++
			if event.Err != nil {
				panic(event.Err)
			}

			fmt.Printf("You pressed: %q\n", event.Rune)
			if event.Key == keyboard.KeyEsc {
				return
			}

			go func(key rune) {
				if err := keys.Run(key, c, op); err != nil {
					panic(err)
				}
			}(event.Rune)
		}
	}
}
