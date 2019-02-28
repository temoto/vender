#ifndef INCLUDE_BIT_H
#define INCLUDE_BIT_H

#define bit_mask_test(x, m) (((x) & (m)) == (m))
#define bit_mask_clear(x, m) ((x) &= ~(m))
#define bit_mask_set(x, m) ((x) |= (m))

#endif  // INCLUDE_BIT_H
