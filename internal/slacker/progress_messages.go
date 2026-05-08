package slacker

var progressMessages = []string{
	"Consulting the oracles",
	"Decoding latent space",
	"Reticulating splines",
	"Feeding tokens to the neural beast",
	"Polishing my silicon brain",
	"Un hallucinating...",
	"Adjusting attention heads",
	"Squeezing gradients through the bottleneck",
	"Computing meaning from noise",
	"Arranging transformers in optimal formation",
	"Counting parameters in my sleep",
	"Simulating consciousness (slowly)",
	"Updating weights, one gradient at a time",
	"Indexing the vast emptiness of knowledge",
	"Remembering to pretend I'm intelligent",
}

var progressEmojis = []string{
	"🕐",
	"💭",
	"⏳",
	"🧠",
	"🤖",
	"⚙️",
	"🔮",
	"📊",
}

type ProgressMessageProvider struct {
	messageIndex int
	emojiIndex   int
}

func (p *ProgressMessageProvider) GetNextMessage() string {
	msg := progressMessages[p.messageIndex]
	p.messageIndex = (p.messageIndex + 1) % len(progressMessages)
	return msg
}

func (p *ProgressMessageProvider) GetNextEmoji() string {
	emoji := progressEmojis[p.emojiIndex]
	p.emojiIndex = (p.emojiIndex + 1) % len(progressEmojis)
	return emoji
}
