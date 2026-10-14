/* Characterization harness: dumps render + hl for every row after edits. */
#define main kilo_main
#include "../oss-src/kilo/kilo.c"
#undef main
static void dump(FILE *o){ for(int r=0;r<E.numrows;r++){ fwrite(E.row[r].render,1,E.row[r].rsize,o); fputc('|',o); fwrite(E.row[r].hl,1,E.row[r].rsize,o); fprintf(o,"|%d|%d\n",E.row[r].hl_oc,E.row[r].size);} }
int main(int argc,char**argv){
  E.cx=E.cy=E.rowoff=E.coloff=E.numrows=0;E.row=NULL;E.dirty=0;E.filename=NULL;E.syntax=NULL;
  E.screenrows=20;E.screencols=40;
  editorSelectSyntaxHighlight(argv[1]); editorOpen(argv[1]);
  FILE*o=fopen(argv[2],"w"); dump(o);
  /* edits: insert chars beyond EOF, newline, deletes */
  E.cy=5;E.cx=3; const char*s="/* x \"y\" 12.5 if(*/\tint"; for(const char*c=s;*c;c++) editorInsertChar(*c);
  editorInsertNewline(); editorDelChar(); editorDelChar();
  E.rowoff=E.numrows+2;E.cy=0;E.cx=7; editorInsertChar('q'); editorInsertNewline();
  E.rowoff=0;E.cy=10;E.cx=0; editorDelChar();
  dump(o); fprintf(o,"dirty=%d rows=%d\n",E.dirty,E.numrows); fclose(o); return 0; }
