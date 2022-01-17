package main

import (
	"flag"
	"os"
	"os/signal"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/khaliullov/coinbasevwap/internal/clients"
	"github.com/khaliullov/coinbasevwap/internal/consumers"
	"github.com/khaliullov/coinbasevwap/internal/entity"
	"github.com/khaliullov/coinbasevwap/internal/producers"
	"github.com/khaliullov/coinbasevwap/internal/repository"
	"github.com/khaliullov/coinbasevwap/internal/usecase"
)

const (
	// DefaultProducts – default trading pairs to subscribed to
	DefaultProducts = "BTC-USD,ETH-USD,ETH-BTC"
	// DefaultVolumeSize – maximum storage size of transactions for each trading pair
	DefaultVolumeSize = 200
)

func getConfig(logger *log.Logger) *entity.Config {
	help := flag.Bool("help", false, "Show help")
	logLevel := flag.String("log-level", "error", "Logging level")
	products := flag.String("products", DefaultProducts, "Products to subscribe to")
	channel := flag.String("channel", clients.DefaultCoinbaseRateFeedChannel, "Channel to subscribe to")
	feedURL := flag.String("feed-url", clients.DefaultCoinbaseRateFeedWebsocketURL, "Coinbase feed URL")
	capacity := flag.Int("capacity", DefaultVolumeSize, "Capacity for storing data for VWAP calculation")

	flag.Parse()

	if *help || *products == "" || *channel == "" || *feedURL == "" || *capacity == 0 || *logLevel == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		level = log.ErrorLevel
	}
	log.SetLevel(level)

	cfg := entity.Config{
		Channels:   strings.Split(*channel, ","),
		ProductIDs: strings.Split(*products, ","),
		URL:        *feedURL,
		Capacity:   *capacity,
	}

	return &cfg
}

func main() {
	logger := log.New()
	logger.SetOutput(os.Stderr)
	logger.Info("Coinbase rate VWAP.")
	logger.SetLevel(log.ErrorLevel)

	cfg := getConfig(logger)

	wg := sync.WaitGroup{}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	client, err := clients.NewCoinbaseRateFeed(logger, &wg, cfg)
	if err != nil {
		log.Fatal(err)
	}

	repo := repository.NewRepository(cfg)
	producer := producers.NewProducer()
	useCase := usecase.NewUseCase(repo, producer, cfg)

	matchConsumer := consumers.NewMatchConsumer(useCase, cfg)
	client.RegisterMatchConsumer(matchConsumer)

	client.Run()

	go func() {
		for {
			if x := <-interrupt; x != nil {
				log.Info("interrupt")
				client.Stop()
				return
			}
		}
	}()

	wg.Wait()
	log.Debug("Finished.")
}
