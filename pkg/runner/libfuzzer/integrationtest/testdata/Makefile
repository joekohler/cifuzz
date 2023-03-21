CC = clang
CXX = clang++

# Keep this flags in sync with the flags used in the make integration
# see also `internal/cmd/run/run.go`
CFLAGS = -fsanitize=fuzzer,address,undefined \
		-fno-sanitize=function,vptr \
		-fsanitize-recover=address \
		-fsanitize-address-use-after-scope \
		-g

CXXFLAGS = -fsanitize=fuzzer,address,undefined \
		-fno-sanitize=function,vptr \
		-fsanitize-recover=address \
		-fsanitize-address-use-after-scope \
		-std=c++17 \
		-g

LDFLAGS =

BUILD_DIR = build
FUZZ_TARGET_DIR = fuzz-targets
# get all c/cpp files and remove file extension
EXT :=
DEPS :=
OUT_OPTION := -o 
INCLUDE_OPTION := -I 
NOLINK_OPTION := -c
OUT_OBJ_OPTION := -o
DISABLE_UBSAN := 0
DEPS := dumper.o
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	LDFLAGS = -Wl,--wrap=__sanitizer_set_death_callback
endif

ALL_PREREQUISITES = clean prepare $(FUZZ_TARGETS)

ifeq ($(DISABLE_UBSAN), 0)
	 ALL_PREREQUISITES += $(FUZZ_TARGET_DIR)/trigger_ubsan_non_recoverable;
	 FUZZ_TARGETS = $(shell find $(FUZZ_TARGET_DIR) -name '*.c*' | cut -f 1 -d '.')
else
	 FUZZ_TARGETS = $(shell find $(FUZZ_TARGET_DIR) -name '*.c*' ! -name '*ubsan*' | cut -f 1 -d '.')
endif

clean:
	-rm -rf $(BUILD_DIR)/*

all: $(ALL_PREREQUISITES)

ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

dumper.o:
	$(CC) -fPIC -c $(ROOT_DIR)/dumper/dumper.c -o $@

$(FUZZ_TARGET_DIR)/trigger_asan_with_file_accesses: OUT=$(BUILD_DIR)/$(notdir $@)$(EXT)
$(FUZZ_TARGET_DIR)/trigger_asan_with_file_accesses: ${FUZZ_TARGET_DIR}/trigger_asan_with_file_accesses.c ${DEPS}
	mkdir -p $(BUILD_DIR)
	$(CC) $^ $(CFLAGS) $(OUT_OPTION)$(OUT) $(LDFLAGS)
	@echo "--> done: $(OUT)"

$(FUZZ_TARGET_DIR)/trigger_ubsan_non_recoverable: OUT=$(BUILD_DIR)/$(notdir $@)$(EXT)
$(FUZZ_TARGET_DIR)/trigger_ubsan_non_recoverable: $(FUZZ_TARGET_DIR)/trigger_ubsan.cpp ${DEPS}
	mkdir -p $(BUILD_DIR)
	$(CXX) $^ $(CXXFLAGS) -fno-sanitize-recover=undefined $(OUT_OPTION)$(OUT) $(LDFLAGS)
	@echo "--> done: $(OUT)"

$(FUZZ_TARGET_DIR)/%: OUT=$(BUILD_DIR)/$(notdir $@)$(EXT)
$(FUZZ_TARGET_DIR)/%: $(FUZZ_TARGET_DIR)/%.c ${DEPS}
	mkdir -p $(BUILD_DIR)
	$(CC) $^ $(CFLAGS) $(OUT_OPTION)$(OUT) $(LDFLAGS)
	@echo "--> done: $(OUT)"

$(FUZZ_TARGET_DIR)/%: OUT=$(BUILD_DIR)/$(notdir $@)$(EXT)
$(FUZZ_TARGET_DIR)/%: $(FUZZ_TARGET_DIR)/%.cpp ${DEPS}
	mkdir -p $(BUILD_DIR)
	$(CXX) $^ $(CXXFLAGS) $(OUT_OPTION)$(OUT) $(LDFLAGS)
	@echo "--> done: $(OUT)"
