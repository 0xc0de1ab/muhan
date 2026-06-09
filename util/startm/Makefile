CFLAGS = -g -O2 
LIBS = -lm 
CC = cc
OUTFILE = startm

OBJECTS = startm.c

startm: startm.c
	$(CC) $(CFLAGS) $(OBJECTS) -o $(OUTFILE) $(LIBS)
	chmod 700 $(OUTFILE)
	strip $(OUTFILE)

# Need to add options to make utils here too
clean: 
	rm $(OBJECTS)

