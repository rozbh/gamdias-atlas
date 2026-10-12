package anim

// Animations is what `atlas fun` plays, in order. A frame is three digits, the only
// shapes the display has, with an optional dot. `?` is a random digit, a trailing
// `!` makes the display flash, and `*N` holds the frame for N ticks.
// Add your own lines here.
var Animations = []Animation{
	// 0 is an empty box and 8 a full one
	{"fill", "000 800 880 888*3 088 008 000"},
	{"ball", "800 080 008 080 800 080 008 080"},
	// 1 7 4 9 8 light more and more segments
	{"wave", "174 749 498 989 894 947 471 717 174 749 498 989 894 947 471 717"},
	{"slot", "???*12 7??*8 77?*8 777!*16"},
	{"launch", "333*5 222*5 111*5 000!*12"},
	{"sos", "505!*16"},
	// BOO BOB BIG ZOO TOY
	{"words", "800*7 808*7 816*7 200*7 704*7"},
	{"eyes", "606*4 909*4 606*4 909*4"},
	{"pi", "314*3 141*3 415*3 159*3 592*3 926*3 265*3 653*3 535*3"},
}
