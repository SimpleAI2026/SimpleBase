#ifndef EM_PORT_API
#	if defined(__EMSCRIPTEN__)
#		include <emscripten.h>
#		if defined(__cplusplus)
#			define EM_PORT_API(rettype) extern "C" rettype EMSCRIPTEN_KEEPALIVE
#		else
#			define EM_PORT_API(rettype) rettype EMSCRIPTEN_KEEPALIVE
#		endif
#	else
#		if defined(__cplusplus)
#			define EM_PORT_API(rettype) extern "C" rettype
#		else
#			define EM_PORT_API(rettype) rettype
#		endif
#	endif
#endif

#include <stdio.h>

int fib(int i) {
		if (i < 2) {
			return i;
		}
		return fib(i - 1) + fib(i - 2);
}

EM_PORT_API(int) sum(int x, int y) {
  return x + y;
}

EM_PORT_API(int) fib_result(int x) {
  return fib(x);
}
