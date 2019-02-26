#!/usr/bin/env awk -f
# Parse MDB bytes csv to human readable command-response sequences.
# We get this CSV from digital scope.

# Example input:
# 3.235384000,MT,MTT,1E0
# 3.236550000,MT,MTT,E0
# 3.237754000,MR,MRR,100

BEGIN {
  FS = ",";
  lastTime = 0; sid = 0; addr = 0;
  lastBus = "";
  mlen = 0; rlen = 0;
  mchk = 0; rchk = 0;
  lastByte = 0;
  cmdByte = 0;
  data = ""; byte = 0;
  line = ""; sdesc = "";
  devName[0x08] = "coin";
  devName[0x30] = "bill";
  devName[0x40] = "hopper1";
  devName[0xc0] = "valve";
  devName[0xc8] = "mixer";
  devName[0xd0] = "elevatr";
  devName[0xd8] = "conveyr";
  devName[0xe0] = "cup";
  devName[0xe8] = "coffee";
  cmdBitName[0] = "reset";
  cmdBitName[1] = "setup";
  cmdBitName[3] = "poll";
  cmdName["c201"] = "runhot";
  cmdName["c202"] = "runcold";
  cmdName["c21000"] = "cold-st";
  cmdName["c21001"] = "cold-on";
  cmdName["c21100"] = "hot-st";
  cmdName["c21101"] = "hot-on";
  cmdName["c21200"] = "boil-st";
  cmdName["c21201"] = "boil-on";
  cmdName["c21400"] = "pump-st";
  cmdName["c21401"] = "pump-on";
  cmdName["c402"] = "errcode";
  cmdName["c411"] = "gettemp";
  cmdName["c510"] = "settemp";
  cmdName["ca01"] = "shaker";
  cmdName["ca02"] = "fan";
  cmdName["ca03"] = "move";
  cmdName["d203"] = "move";
  cmdName["da01"] = "move";
  cmdName["da03"] = "shake";
  cmdName["dd10"] = "setspd";
  cmdName["e201"] = "dispens";
  cmdName["e202"] = "lighton";
  cmdName["e203"] = "lightof";
  cmdName["e204"] = "check";
  cmdName["e402"] = "errcode";
  cmdName["ea01"] = "grind";
  cmdName["ea02"] = "press";
  cmdName["ea03"] = "dispose";
  cmdName["ea05"] = "heat-on";
  cmdName["ea06"] = "heat-st";
}

function parseTime(s) {
  isms = sub("ms", "", s) > 0;
  isrel = sub("+", "", s) > 0;
  v = strtonum(s);
  if (isms) { v /= 1000 }
  if (isrel) { v += lastTime }
  return v
}
function parseAddr(di) { return and(di, 0xf8) }
function low(a) { return and(a, 0xff) }
function printLine() {
  if (line) {
    print line "\t" commandName() "\t" sdesc;
  }
  line = ""; sdesc = "";
}
function deviceName(addr) {
  x = devName[addr];
  if (!x) { x = sprintf("%02x ?", addr) }
  return x;
}
function commandName() {
  split(sdesc, parts, "->")
  mts = parts[1]
  gsub(" ", "", mts)
  if (x = cmdName[substr(mts, 1, 6)]) { return x }
  if (x = cmdName[substr(mts, 1, 4)]) { return x }
  if (x = cmdName[substr(mts, 1, 2)]) { return x }
  cmdBits = and(low(cmdByte), 0x7);
  x = cmdBitName[cmdBits];
  if (x) { return x }
  return sprintf("%d ?", cmdBits);
}

/^Time/ { next }
/Error$/ { next }

{
lastBus = bus;
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
