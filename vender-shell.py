#!/usr/bin/env python
# coding: utf-8
from __future__ import print_function
import functools,pigpio,readline,struct,sys,time

pi = pigpio.pi()
i2c = pi.i2c_open(0, 0x78)

def crc8_p93(crc, data):
  crc ^= data
  for i in xrange(8):
    if (crc&0x80)!=0:
      crc = (crc<<1)&0xff
      crc ^= 0x93
    else:
      crc = (crc<<1)&0xff
  return crc

def crc8_p93_bytes(bs):
  return functools.reduce(crc8_p93, map(ord, bs), 0)

def i2c_tx(send='', echo_out=True, echo_in=True):
  bs = send.decode('hex')
  if len(bs) > 0:
    if echo_out:
      print('> ' + send)
    pi.i2c_zip(i2c, [4, 0x78, 7, len(bs)] + list(map(ord,bs)) + [0])
  _, s = pi.i2c_zip(i2c, [4, 0x78, 6, max(len(bs), 73), 0])
  if echo_in:
    print('< ' + str(s).encode('hex'))
  return s

HEADER_CODE_MAP = {
  0x01: 'OK',
  0x02: 'Config',
  0x04: 'Debug',
  0x05: 'TWI',
  0x08: 'MDB-started',
  0x09: 'MDB-success',
  0x80: 'Error',
  0x81: 'Err-bad-packet',
  0x82: 'Err-CRC8',
  0x83: 'Err-buffer-overflow',
  0x84: 'Err-unknown-command',
  0x85: 'Err-corruption',
  0x88: 'Err-MDB-busy',
  0x89: 'Err-MDB-protocol',
  0x8a: 'Err-MDB-CHK',
  0x8b: 'Err-MDB-NACK',
  0x8c: 'Err-MDB-timeout',
  0x90: 'Err-UART-chatterbox',
  0x91: 'Err-UART-read',
}

def slave_talk(send='', **kw):
  if send.startswith('!'):
    send = send[1:]
  elif send:
    bs = send.decode('hex')
    send_length = len(bs) + 2
    bs = chr(send_length) + bs
    crc = crc8_p93_bytes(bs)
    bs += chr(crc)
    send = bs.encode('hex')

  kw.setdefault('echo_in', False)
  result = i2c_tx(send, **kw)
  queue = 0
  while result and result[0] not in (0,0xff):
    length = result[0]
    header = result[1]
    data = str(result[2:length-1])
    header_str = HEADER_CODE_MAP.get(header, 'UNKNOWN')
    print('< ' + str(result[:length]).encode('hex'))
    if header == 0x01 and data:
      queue = ord(data[0])
      info = 'queue: {0:d}'.format(queue)
    else:
      info = '({0})'.format(data)
    print('{0} {1} {2}'.format(header_str, data.encode('hex'), info))
    result = result[length:]
  return queue

def slave_shell():
  print('Hello. You can use TAB and type "help".')
  while True:
    try:
      s = raw_input('? ')
    except EOFError:
      break
    if s == 'q':
      break
    if s == 'help':
      print('''
Each line can contain zero or more commands separated by space.

Safe command is just hex. Examples: 01, 0fcb. AVR-UART packet is framed automatically.
Raw commands start with !. Examples: !03013b, !040fcbff. Send to TWI as-is, no length or CRC8 is added.
Sleep starts with 's' then time in milliseconds. Examples: s10, s200.

Complex input example:
input: 01 s100 04 s30 0fcb s30 01 0f30 s30 01 04
what it does:
- status poll
- wait 100ms
- read debug log
- wait 30ms
- MDB send '1cb cb', wait 30ms, status poll
- MDB send '130 30', wait 30ms, status poll
- read debug log

Commands:
- 01 status poll, no data
- 02 update config, slave returns full config in response
- 03 reset, no data, wait 100ms then expect 0600beebee(CRC) from slave
- 04 read debug info
- 07 MDB bus reset (hold TX high for 100ms)
- 08-0f MDB transaction
  bit0 add auto CHK
  bit1 verify response CHK
  bit2 repeat on timeout
  useful values:
  08 (debug) your CHK, ignore response CHK, no repeat
  0f (release) auto add and verify CHK, repeat on timeout
''')
      continue
    if s == '':
      s = '01'
    parts = s.split()
    for part in parts:
      if part.startswith('s'):
        duration = float(part[1:])
        print('S {}ms'.format(duration))
        time.sleep(duration/1000)
      else:
        q = slave_talk(part)
        if part not in ('', '01'):
          q = slave_talk()
        if q > 0:
          slave_talk()

def readline_complete(text, state):
  root = [
    '!',
    '01', '02', '03', '04', '07', '08', '0f',
    'help',
    's',
  ]
  possible = [s for s in root if s.startswith(text)]
  x = possible[state:state+1]
  if x:
    return x[0]
  return None

if __name__ == '__main__':
  import os,readline
  history_path = os.path.expanduser('~/.vender-shell-history')
  try:
    readline.read_history_file(history_path)
  except IOError:
    pass
  readline.set_history_length(1000)
  readline.parse_and_bind('tab: complete')
  readline.set_completer(readline_complete)
  slave_shell()
  readline.write_history_file(history_path)
