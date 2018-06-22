#include <assert.h>

int main() {
  Init();
  Poll_Loop(0);
  Master_Out_Printf(Response_Debug, "");
  return 0;
}
