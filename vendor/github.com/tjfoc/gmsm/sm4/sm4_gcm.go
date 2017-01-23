/*
Copyright Hyperledger-TWGC All Rights Reserved.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

                 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

writed by Zhiwei Yan, 2020 Oct
*/
package sm4

import (
	"errors"
	"strconv"
)

//Paper: The Galois/Counter Mode of Operation (GCM) David A. Mcgrew，John Viega .2004.
func Sm4GCM(key []byte, IV ,in, A []byte, mode bool) ([]byte, []byte, error) {
	if len(key) != BlockSize {
		return nil,nil, errors.New("SM4: invalid key size " + strconv.Itoa(len(key)))
	}
	if mode {
		C,T:=GCMEncrypt(key,IV,in,A)
		return C,T,nil
	}else{
		P,_T:=GCMDecrypt(key,IV,in,A)
		return P,_T,nil
	}
}

func GetH(key []byte) (H []byte){
	c,err := NewCipher(key)
	if err != nil {
		panic(err)
	}

	zores:=make([]byte, BlockSize)
	H =make([]byte, BlockSize)
	c.Encrypt(H,zores)
	return H
}

//ut = a + b
func addition(a ,b  []byte) (out []byte){
	Len:=len(a)
	if Len != len(b) {
		return nil
	}
	out = make([]byte, Len)
	for i := 0; i < Len; i++ {
		out[i] = a[i] ^ b[i]
	}
	return out
}

func Rightshift(V []byte){
	n:=len(V)
	for i:=n-1;i>=0;i-- {
		V[i]=V[i]>>1
		if i!=0{
			V[i]=((V[i-1]&0x01)<<7)|V[i]
		}
	}
}

func findYi( Y []byte,index int) int{
	var temp byte
	i := uint(index)
	temp=Y[i/8]
	temp=temp>>(7-i%8)
	if temp & 0x01 == 1{
		return 1
	}else{
		return 0
	}
}


func multiplication(X,Y []byte) (Z []byte){

	R:=make([]byte,BlockSize)
	R[0]=0xe1
	Z=make([]byte,BlockSize)
	V:=make([]byte,BlockSize)
	copy(V,X)
	for i:=0;i<=127;i++{
		if findYi(Y,i)==1{
			Z=addition(Z,V)
		}
		if V[BlockSize-1]&0x01==0{
			Rightshift(V)
		}else{
			Rightshift(V)
			V=addition(V,R)
		}
	}
	return Z
}

func GHASH(H []byte,A []byte,C []byte) (X[]byte){

	calculm_v:=func(m ,v int) (int,int) {
		if(m==0 && v!=0){
			m=1
			v=v*8
		}else if(m!=0 && v==0) {
			v=BlockSize*8
		}else if(m!=0 && v!=0){
			m=m+1
			v=v*8
		}else { //m==0 && v==0
			m=1
			v=0
		}
		return m,v
	}
	m:=len(A)/BlockSize
	v:=len(A)%BlockSize
	m,v=calculm_v(m,v)

	n:=len(C)/BlockSize
	u:=(len(C)%BlockSize)
	n,u=calculm_v(n,u)

	//i=0
	X=make([]byte,BlockSize*(m+n+2)) //X0 = 0
	for i:=0;i<BlockSize;i++{
		X[i]=0x00
	}

	//i=1...m-1
	for i:=1;i<=m-1;i++{
		copy(X[i*BlockSize:i*BlockSize+BlockSize],multiplication(addition(X[(i-1)*BlockSize:(i-1)*BlockSize+BlockSize],A[(i-1)*BlockSize:(i-1)*BlockSize+BlockSize]),H)) //A 1-->m-1 对于数组来说是 0-->m-2
	}

	//i=m
	zeros:=make([]byte,(128-v)/8)
	Am:=make([]byte,v/8)
	copy(Am[:],A[(m-1)*BlockSize:])
	Am=append(Am,zeros...)
	copy(X[m*BlockSize:m*BlockSize+BlockSize],multiplication( addition(X[(m-1)*BlockSize:(m-1)*BlockSize+BlockSize],Am),H))

	//i=m+1...m+n-1
	for i:=m+1;i<=(m+n-1);i++{
		copy(X[i*BlockSize:i*BlockSize+BlockSize],multiplication( addition(X[(i-1)*BlockSize:(i-1)*BlockSize+BlockSize],C[(i-m-1)*BlockSize:(i-m-1)*BlockSize+BlockSize]),H))
	}

	//i=m+n
	zeros =make([]byte,(128-u)/8)
	Cn:=make([]byte,u/8)
	copy(Cn[:],C[(n-1)*BlockSize:])
	Cn=append(Cn,zeros...)
	copy(X[(m+n)*BlockSize:(m+n)*BlockSize+BlockSize],multiplication( addition(X[(m+n-1)*BlockSize:(m+n-1)*BlockSize+BlockSize],Cn),H))

	//i=m+n+1
	var lenAB []byte
	calculateLenToBytes :=func(len int) []byte{
		data:=make([]byte,8)
		data[0]=byte((len>>56)&0xff)
		data[1]=byte((len>>48)&0xff)
		data[2]=byte((len>>40)&0xff)
		data[3]=byte((len>>32)&0xff)
		data[4]=byte((len>>24)&0xff)
		data[5]=byte((len>>16)&0xff)
		data[6]=byte((len>>8)&0xff)
		data[7]=byte((len>>0)&0xff)
		return data
	}
	lenAB=append(lenAB,calculateLenToBytes(len(A))...)
	lenAB=append(lenAB,calculateLenToBytes(len(C))...)
	copy(X[(m+n+1)*BlockSize:(m+n+1)*BlockSize+BlockSize],multiplication(addition(X[(m+n)*BlockSize:(m+n)*BlockSize+BlockSize],lenAB),H))
	return  X[(m+n+1)*BlockSize:(m+n+1)*BlockSize+BlockSize]
}


func GetY0(H,IV []byte) []byte{
	if len(IV)*8 == 96 {
		zero31one1:=[]byte{0x00,0x00,0x00,0x01}
		IV=append(IV,zero31one1...)
		return IV
	}else{
		return GHASH(H,[]byte{},IV)

	}

}

func incr(n int ,Y_i []byte) (Y_ii []byte) {

	Y_ii=make([]byte,BlockSize*n)
	copy(Y_ii,Y_i)

	addYone:=func(yi,yii []byte){
		copy(yii[:],yi[:])

		Len:=len(yi)
		var rc byte=0x00
		for i:=Len-1;i>=0;i--{
			if(i==Len-1){
				if(yii[i]<0xff){
					yii[i]=yii[i]+0x01
					rc=0x00
				}else{
					yii[i]=0x00
					rc=0x01
				}
			}else{
				if yii[i]+rc<0xff {
					yii[i]=yii[i]+rc
					rc=0x00
				}else{
					yii[i]=0x00
					rc=0x01
				}
			}
		}
	}
	for i:=1;i<n;i++{ //2^32
		addYone(Y_ii[(i-1)*BlockSize:(i-1)*BlockSize+BlockSize],Y_ii[i*BlockSize:i*BlockSize+BlockSize])
	}
	return Y_ii
}

func MSB(len int, S []byte) (out []byte){
	return S[:len/8]
}
func GCMEncrypt(K,IV,P,A []byte) (C,T []byte){
	calculm_v:=func(m ,v int) (int,int) {
		if(m==0 && v!=0){
			m=1
			v=v*8
		}else if(m!=0 && v==0) {
			v=BlockSize*8
		}else if(m!=0 && v!=0){
			m=m+1
			v=v*8
		}else { //m==0 && v==0
			m=1
			v=0
		}
		return m,v
	}
	n:=len(P)/BlockSize
	u:=len(P)%BlockSize
	n,u=calculm_v(n,u)

	H:=GetH(K)

	Y0:=GetY0(H,IV)

	Y:=make([]byte,BlockSize*(n+1))
	Y=incr(n+1,Y0)
	c,err := NewCipher(K)
	if err != nil {
		panic(err)
	}
	Enc:=make([]byte,BlockSize)
	C =make([]byte,len(P))

	//i=1...n-1
	for i:=1;i<=n-1;i++{
		c.Encrypt(Enc,Y[i*BlockSize:i*BlockSize+BlockSize])

		copy(C[(i-1)*BlockSize:(i-1)*BlockSize+BlockSize],addition(P[(i-1)*BlockSize:(i-1)*BlockSize+BlockSize],Enc))
	}

	//i=n
	c.Encrypt(Enc,Y[n*BlockSize:n*BlockSize+BlockSize])
	out:=MSB(u,Enc)
	copy(C[(n-1)*BlockSize:],addition(P[(n-1)*BlockSize:],out))

	c.Encrypt(Enc,Y0)

	t:=128
	T =MSB(t,addition(Enc,GHASH(H,A,C)))
	return C,T
}

func GCMDecrypt(K,IV,C,A []byte)(P,_T []byte){
	calculm_v:=func(m ,v int) (int,int) {
		if(m==0 && v!=0){
			m=1
			v=v*8
		}else if(m!=0 && v==0) {
			v=BlockSize*8
		}else if(m!=0 && v!=0){
			m=m+1
			v=v*8
		}else { //m==0 && v==0
			m=1
			v=0
		}
		return m,v
	}

	H:=GetH(K)

	Y0:=GetY0(H,IV)

	Enc:=make([]byte,BlockSize)
	c,err := NewCipher(K)
	if err != nil{
		panic(err)
	}
	c.Encrypt(Enc,Y0)
	t:=128
	_T=MSB(t,addition(Enc,GHASH(H,A,C)))

	n:=len(C)/BlockSize
	u:=len(C)%BlockSize
	n,u=calculm_v(n,u)
	Y:=make([]byte,BlockSize*(n+1))
	Y=incr(n+1,Y0)

	P = make([]byte, BlockSize*n)
	for i:=1;i<=n;i++{
		c.Encrypt(Enc,Y[i*BlockSize:i*BlockSize+BlockSize])
		copy(P[(i-1)*BlockSize:(i-1)*BlockSize+BlockSize],addition(C[(i-1)*BlockSize:(i-1)*BlockSize+BlockSize],Enc))
	}

	c.Encrypt(Enc,Y[n*BlockSize:n*BlockSize+BlockSize])
	out:=MSB(u,Enc)
	copy(P[(n-1)*BlockSize:],addition(C[(n-1)*BlockSize:],out))

	return P,_T
}
