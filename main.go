package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/liteclient"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/ton/wallet"
	"github.com/xssnick/tonutils-go/tvm/cell"
)

var (
	client *liteclient.ConnectionPool
	api    *ton.APIClient
	w      *wallet.Wallet
)

func main() {
	initializeApp()

	http.HandleFunc("/sendTransactions", sendTransactionsHandler)
	http.ListenAndServe(":8888", nil)
}

func initializeApp() {
	var err error
	client = liteclient.NewConnectionPool()

	err = client.AddConnection(context.Background(), "135.181.140.212:13206", "K0t3+IWLOXHYMvMcrGZDPs+pn58a17LFbnXoQkKc2xw=")
	if err != nil {
		log.Fatalln("connection err: ", err.Error())
		return
	}

	api = ton.NewAPIClient(client)

	err = godotenv.Load()
	if err != nil {
		log.Fatalln("Error loading .env file", err.Error())
	}

	seedWords := os.Getenv("SEED_PHRASE")
	var words []string

	if seedWords == "" {
		log.Fatalln("SEED_PHRASE env is empty")
		words = wallet.NewSeed()
		log.Println("Generated seed words:", strings.Join(words, " "))
	} else {
		words = strings.Split(seedWords, " ")
	}

	w, err = wallet.FromSeed(api, words, wallet.HighloadV2R2)
	if err != nil {
		log.Fatalln("FromSeed err:", err.Error())
		return
	}

	log.Println("Wallet address:", w.Address())
}

func sendTransactionsHandler(responseWriter http.ResponseWriter, req *http.Request) {
	sendMode, err := getSendMode(req)
	if err != nil {
		respondError(responseWriter, err)
		return
	}

	commentText := req.URL.Query().Get("comment")
	log.Println("comment:", commentText)

	txs, err := getTransactions(req)
	if err != nil {
		respondError(responseWriter, err)
		return
	}

	err = processTransactions(responseWriter, sendMode, commentText, txs)
	if err != nil {
		respondError(responseWriter, err)
	}
}

func getSendMode(req *http.Request) (uint64, error) {
	sendModeString := req.URL.Query().Get("send_mode")
	log.Println("Send mode:", sendModeString)

	return strconv.ParseUint(sendModeString, 10, 8)
}

func getTransactions(req *http.Request) (map[string]string, error) {
	var txs map[string]string
	err := json.NewDecoder(req.Body).Decode(&txs)

	if err != nil {
		log.Println("Json decode err:", err.Error())
		return nil, err
	}

	log.Println("Transactions:", txs)
	return txs, nil
}

func processTransactions(responseWriter http.ResponseWriter, sendMode uint64, commentText string, txs map[string]string) error {
	block, err := api.CurrentMasterchainInfo(context.Background())
	if err != nil {
		return err
	}

	balance, err := w.GetBalance(context.Background(), block)
	if err != nil {
		return err
	}

	totalAmount, err := calculateTotalAmount(txs)
	if err != nil {
		return err
	}

	if balance.NanoTON().Uint64() >= totalAmount {
		comment, err := wallet.CreateCommentCell(commentText)
		if err != nil {
			return err
		}

		messages := createMessages(sendMode, comment, txs)

		log.Println("Sending transaction and waiting for confirmation...")

		txHash, err := w.SendManyWaitTxHash(context.Background(), messages)
		if err != nil {
			return err
		}

		return sendSuccessResponse(responseWriter, txHash)
	}

	return notEnoughBalanceError()
}

func calculateTotalAmount(txs map[string]string) (uint64, error) {
	var totalAmount float64
	for _, amtStr := range txs {
		amtFloat, err := strconv.ParseFloat(amtStr, 64)
		if err != nil {
			return 0, err
		}
		totalAmount += amtFloat
	}

	return uint64(totalAmount * 1e9), nil
}

func createMessages(sendMode uint64, comment *cell.Cell, txs map[string]string) []*wallet.Message {
	var messages []*wallet.Message
	for addrStr, amtStr := range txs {
		messages = append(messages, &wallet.Message{
			Mode: uint8(sendMode),
			InternalMessage: &tlb.InternalMessage{
				Bounce:  false,
				DstAddr: address.MustParseAddr(addrStr),
				Amount:  tlb.MustFromTON(amtStr),
				Body:    comment,
			},
		})
	}
	return messages
}

func sendSuccessResponse(responseWriter http.ResponseWriter, txHash []byte) error {
	log.Println("Transaction sent, hash:", base64.StdEncoding.EncodeToString(txHash))
	log.Println("Explorer link: https://tonscan.org/tx/" + base64.URLEncoding.EncodeToString(txHash))

	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusOK)
	return json.NewEncoder(responseWriter).Encode(map[string]string{
		"txHash": base64.StdEncoding.EncodeToString(txHash),
		"link":   "https://tonscan.org/tx/" + base64.URLEncoding.EncodeToString(txHash),
	})
}

func notEnoughBalanceError() error {
	return &customError{"Not enough balance"}
}

type customError struct {
	msg string
}

func (e *customError) Error() string {
	return e.msg
}

func respondError(responseWriter http.ResponseWriter, err error) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(responseWriter).Encode(map[string]string{
		"error": err.Error(),
	})
}
