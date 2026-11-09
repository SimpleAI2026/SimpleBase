#define main kilo_main
#include "input.c"
#undef main
int main(void){ E.numrows=0;E.row=NULL;E.syntax=NULL;
 for(int i=0;i<4;i++) editorInsertRow(E.numrows,"x",1);
 editorDelRow(1);
 for(int j=0;j<E.numrows;j++) printf("row %d idx=%d\n",j,E.row[j].idx); return 0;}
