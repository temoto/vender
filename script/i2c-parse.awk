#!/usr/bin/env awk -f
# Parse mega bytes csv to human readable command-response sequences.
# We get this CSV from digital scope.

# Example input:
# Time(seconds),Bus Name,Signal Name,Data
# 3.649978000,I2C 3,Digital 3,S - Start
# 3.649995000,I2C 3,Digital 3,F0 Write
# 3.650073000,I2C 3,Digital 3,ACK
# 3.650089000,I2C 3,Digital 3,05

BEGIN {
  FS = ",";
  lastTime = 0; sid = 0;
  mlen = 0; rlen = 0;
  lastByte = 0;
  cmdByte = 0;
  data = ""; byte = 0;
  line = ""; sdesc = "";
}

function parseTime(s) {
  isms = sub("ms", "", s) > 0;
  isrel = sub("+", "", s) > 0;
  v = strtonum(s);
  if (isms) { v /= 1000 }
  if (isrel) { v += lastTime }
  return v
}
function low(a) { return and(a, 0xff) }
function printLine() {
  if (line) {
    print line "\t" commandName() "\t" sdesc;
  }
  line = ""; sdesc = "";
}

/^Time/ { next }
/Error$/ { next }

{
lastByte = byte;
time = parseTime($1);
bus = $2;
data = $4;
byte = strtonum("0x"data);
byteLoHex = sprintf("%02x ", low(byte));

# special processing of first line
if (lastTime == 0) { lastTime = time; addr = parseAddr(byte); }
if (time - lastTime < 0) { lastTime = time ; lastBus = "" ; lastByte = 0; }

if (bus == "MT") {
  if (byte >= 0x100) {
    # begin new VMC command (session)
    printLine();
    mchk = 0;
    mlen = 0;
    sid += 1;
    cmdByte = byte;
    addr = parseAddr(byte);
    sdesc = byteLoHex;

    offset = (time - lastTime) * 1000;
    line = sprintf("%d\t%.1f\t%d\t%s",
      time*1000, offset, sid, deviceName(addr))
  } else {
    if (lastBus == "MR") {
      # VMC ack/nak of peripheral response
      if (byte == 0x00) {
        sdesc = sdesc "-> ACK "
      } else if (byte == 0x55) {
        sdesc = sdesc "-> RET "
      } else if (byte == 0xff) {
        sdesc = sdesc "-> NAK "
      } else {
        sdesc = sdesc sprintf("ERR(%02x!=ACK/RET/NAK)", byte)
      }
    } else {
      sdesc = sdesc byteLoHex;
    }
  }
  mchk = low(mchk + byte);
  lastTime = time;
} else if (bus == "MR") {
  if (lastBus == "MT") {
    rlen = 0;

    mchkb = lastByte;
    mchk = low(mchk + 0x100 - lastByte);
    # skip valid checksum
    sdesc = substr(sdesc, 1, length(sdesc)-3);
    if (mchk != mchkb) {
      sdesc = sdesc sprintf("CHKERR(%02x!=%02x)", mchkb, mchk);
    }
  }
  rlen += 1;
  lastTime = time;
  if (rlen == 1 && byte >= 0x100) {
    # short peripheral response: ACK/NAK
    if (byte == 0x100) {
      sdesc = sdesc "-> ACK ";
    } else if (byte == 0x1ff) {
      sdesc = sdesc "-> NAK ";
    } else {
      sdesc = sdesc sprintf("-> CHKERR(%02x!=ACK/NAK)", byte)
    }
  } else if (rlen == 1 && byte < 0x100) {
    # begin long peripheral response
    rchk = low(byte);
    sdesc = sdesc "-> " byteLoHex;
  } else if (rlen > 1 && byte < 0x100) {
    # long peripheral response continued
    rchk = low(rchk + byte);
    sdesc = sdesc byteLoHex;
  } else if (rlen > 1 && byte >= 0x100) {
    # long peripheral response checksum
    rchkb = low(byte);
    if (rchk != rchkb) {
      sdesc = sdesc sprintf("CHKERR(%02x!=%02x)", rchkb, rchk);
    }
  }
}
}

END { printLine(); }
