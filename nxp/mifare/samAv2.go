package mifare


import (
	"fmt"
	_ "github.com/ebfe/scard"
	"math/rand"
	"time"
	"github.com/dumacp/smartcard"
	"crypto/aes"
	"crypto/cipher"
	"github.com/aead/cmac"
)

//SamAv2 Interface
type SamAv2 interface {
	smartcard.Card
	GetVersion()	([]byte, error)
	//Connect()	(bool, error)
	AuthHostAV2([]byte, int)	([]byte, error)
	NonXauthMFPf1(first bool, sl, keyNo, keyVer int, data, dataDiv []byte ) ([]byte, error)
	NonXauthMFPf2(data []byte ) ([]byte, error)
	DumpSessionKey()	([]byte, error)
}

type samAv2 struct {
	smartcard.Card
}

//Create SamAv2 interface
func ConnectSamAv2(r smartcard.Reader) (SamAv2, error) {

	c, err := r.ConnectCard()
	if err != nil {
		return nil, err
	}
	sam := &samAv2{
		c,
	}
	return sam, nil
}

//SAM_GetVersion
func (sam *samAv2) GetVersion() ([]byte, error) {
	return sam.Apdu([]byte{0x80,0x60,0x00,0x00,0x00})
}

//SAM_AuthenticationHost AV2 mode
func (sam *samAv2) AuthHostAV2(key []byte, keyNo int) ([]byte, error) {
	rand.Seed(time.Now().UnixNano())
	iv := make([]byte,16)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	modeE := cipher.NewCBCEncrypter(block, iv)
	modeD := cipher.NewCBCDecrypter(block, iv)

	aid1 := []byte{0x80,0xa4,0x00,0x00,0x03,byte(keyNo),0x00,0x00,0x00}

	response, err := sam.Apdu(aid1)
	if err != nil {
		return nil, err
	}
	if response[len(response)-1] != byte(0xAF) {
		return nil, fmt.Errorf("bad formed response: [% X]\n", response)
	}
	rnd2 := response[0:len(response)-2]
	var1 := make([]byte,0)
	var1 = append(var1, rnd2...)
	var1 = append(var1, make([]byte,4)...)
	cmacS, err := cmac.Sum(var1, block, 16)
	if err != nil {
		return nil, err
	}
	cmac2 := make([]byte,0)
	for i, v := range cmacS {
		if i%2 != 0 {
			cmac2 = append(cmac2, v)
		}
	}
	rnd1 := make([]byte,12)
	rand.Read(rnd1)

	/**
	fmt.Printf("cmacS: [% X]\n", cmacS)
	fmt.Printf("cmac2: [% X]\n", cmac2)
	fmt.Printf("rnd1: [% X]\n", rnd1)
	fmt.Printf("rnd2: [% X]\n", rnd2)
	/**/


	aid2 := []byte{0x80,0xa4,0x00,0x00,0x14}
	aid2 = append(aid2, cmac2...)
	aid2 = append(aid2, rnd1...)
	aid2 = append(aid2, byte(0x00))
	fmt.Printf("aid2: [% X]\n", aid2)
	response, err = sam.Apdu(aid2)
	if err != nil {
		return nil, err
	}
	if response[len(response)-1] != byte(0xAF) {
		return nil, fmt.Errorf("bad formed response: [% X]\n", response)
	}

	rndBc := response[8:len(response)-2]

	aXor := make([]byte,5)
	for i, _ := range aXor {
		aXor[i] = rnd1[i] ^ rnd2[i]
	}
	divKey := make([]byte,0)
	divKey = append(divKey, rnd1[7:12]...)
	divKey = append(divKey, rnd2[7:12]...)
	divKey = append(divKey, aXor...)
	divKey = append(divKey, byte(0x91))

	kex := make([]byte,16)
	modeE.CryptBlocks(kex, divKey)

	rndB := make([]byte,len(rndBc))
	block, err = aes.NewCipher(kex)
	if err != nil {
		return nil, err
	}
	modeD = cipher.NewCBCDecrypter(block, iv)
	modeD.CryptBlocks(rndB, rndBc)

	rndA := make([]byte,len(rndB))
	rand.Read(rndA)
	/**
	fmt.Printf("rndA: [% X]\n", rndA)
	fmt.Printf("rndB: [% X]\n", rndB)
	/**/

	//rotate(2)
	rotate := 2
	for i, v := range rndB {
		if i >= rotate {
			break
		}
		rndB = append(rndB,v)
		rndB = rndB[1:]
	}
	//fmt.Printf("rndB2: [% X]\n", rndB)

	rndD := make([]byte,0)
	rndD = append(rndD, rndA...)
	rndD = append(rndD, rndB...)

	//fmt.Printf("rndD: [% X]\n", rndD)

	ecipher := make([]byte, len(rndD))
	modeE = cipher.NewCBCEncrypter(block, iv)
	modeE.CryptBlocks(ecipher, rndD)

	aid3 := []byte{0x80,0xa4,0x00,0x00,0x20}
	aid3 = append(aid3, ecipher...)
	aid3 = append(aid3, byte(0x00))

	response, err = sam.Apdu(aid3)
	if err != nil {
		return nil, err
	}

	if response[len(response)-1] != byte(0x00) || response[len(response)-2] != byte(0x90) {
		return nil, fmt.Errorf("bad formed response: [% X]\n", response)
	}

	return response, nil
}

//SAM_AuthenticationMFP (non-X-mode) first part
func (sam *samAv2) NonXauthMFPf1(first bool, sl, keyNo, keyVer int, data, dataDiv []byte ) ([]byte, error) {
	p1 := byte(0x00)
	if dataDiv != nil {
		p1 = byte(0x01)
	}
	if !first {
		p1 = p1 + byte(0x02)
	}
	if sl == 2 {
		p1 = p1 + byte(0x04)
	} else if sl == 3 {
		p1 = p1 + byte(0x0C)
	} else if sl != 0 {
		p1 = p1 + byte(0x80)
	}

	aid1 := []byte{0x80,0xA3,p1,0x00,byte(18+len(dataDiv))}
	aid1 = append(aid1, byte(keyNo))
	aid1 = append(aid1, byte(keyVer))
	aid1 = append(aid1, data...)

	if dataDiv != nil {
		aid1 = append(aid1, dataDiv...)
	}

	aid1 = append(aid1, byte(0x00))
	return sam.Apdu(aid1)
}


//SAM_AuthenticationMFP (non-X-mode) second part
func (sam *samAv2) NonXauthMFPf2(data []byte ) ([]byte, error) {

	aid1 := []byte{0x80,0xA3,0x00,0x00,byte(len(data))}
	aid1 = append(aid1, data...)
	aid1 = append(aid1, byte(0x00))

	return sam.Apdu(aid1)
}

//SAM_DumpSessionKey (session key of an established authentication with a DESFire or MIFARE Plus PICC)
func (sam *samAv2) DumpSessionKey() ([]byte, error) {
	response, err := sam.Apdu([]byte{0x80,0xD5,0x00,0x00,0x00})
	if err != nil {
		return nil, err
	}
	if response[len(response)-1] != byte(0x00) || response[len(response)-2] != byte(0x90) {
		return nil, fmt.Errorf("bad formed response: [% X]\n", response)
	}

	return response, nil
}


