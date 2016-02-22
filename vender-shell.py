#!/usr/bin/env python
from __future__ import print_function
import functools,pigpio,struct,sys,time

twi_addr = 0x78
pi = pigpio.pi()
i2c = pi.i2c_open(0, twi_addr)

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
    pi.i2c_zip(i2c, [4, twi_addr, 7, len(bs)] + list(map(ord,bs)) + [0])
  _, s = pi.i2c_zip(i2c, [4, twi_addr, 6, max(len(bs), 73), 0])
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
  while result and result[0] not in (0,0xff):
    length = result[0]
    header = result[1]
    data = str(result[2:length-1])
    header_str = HEADER_CODE_MAP.get(header, 'UNKNOWN')
    print('< ' + str(result[:length]).encode('hex'))
    if header == 0x01 and data:
      info = 'queue: {0:d}'.format(ord(data[0]))
    else:
      info = '({0})'.format(data)
    print('{0} {1} {2}'.format(header_str, data.encode('hex'), info))
    result = result[length:]

def slave_shell():
  while True:
    try:
      s = raw_input('? ')
    except EOFError:
      break
    if s == 'q':
      break
    slave_talk(s)

if __name__ == '__main__':
  slave_shell()
