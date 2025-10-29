LLAMA_SERVER_DIR = ../../vendor/llama.cpp/tools/server/

.PHONY: clean all

all: utils.hpp server.cpp

utils.hpp: $(LLAMA_SERVER_DIR)/utils.hpp
	cp $(LLAMA_SERVER_DIR)/utils.hpp .

server.cpp: $(LLAMA_SERVER_DIR)/server.cpp
	cp $(LLAMA_SERVER_DIR)/server.cpp .
	patch server.cpp < server.patch

clean:
	rm *.cpp
	rm *.hpp
