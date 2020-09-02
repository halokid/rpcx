package main

import "testing"

func TestComm(t *testing.T) {
  var bs [8]byte
  // 0x40 转为二进制为  0100 0000
  bs[2] = bs[2] | 0x40

  t.Logf("bs[2] -------------------- %+v", bs[2])
  t.Logf("bs[2] check -------------------- %+v", (bs[2] & 0x40 == 0x40))
  t.Logf("bs[2] check -------------------- %+v", (bs[2] & 0x40 == 0x30))

  //bs[3] = bs[3] & 0x40
  //bs[3] = bs[3] ^ 0x40
  //bs[3] = bs[3] &^ 0x40
  // todo:  a &^ b  的意思是    a & (^b)
  bs[3] = bs[3] &^ 0x40
  t.Logf("bs[3] -------------------- %+v", bs[3])

  t.Logf("^bs[3] -------------------- %+v", ^bs[3])
}