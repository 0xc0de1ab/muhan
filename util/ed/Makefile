CFLAGS = -g -O2 
LIBS = -lm  
CC = cc

OBJECTS = editor.o editor2.o editor3.o editor4.o compress.o

editor: $(OBJECTS)
	$(CC) $(CFLAGS) $(OBJECTS) -o editor $(LIBS)
	chmod 700 editor 
	strip editor

clean: 
	rm $(OBJECTS) $(OBJECTS2)

