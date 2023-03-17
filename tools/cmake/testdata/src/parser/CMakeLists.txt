add_library(secrets secrets.cpp)
target_sources(secrets PUBLIC
               secrets.h)
set_property(TARGET secrets PROPERTY POSITION_INDEPENDENT_CODE ON)
target_include_directories(secrets PUBLIC ${CMAKE_CURRENT_SOURCE_DIR})

if(WIN32)
  # libfuzzer has to be linked statically for clang-cl to get fuzzer
  # instrumentation, but that causes the linker to link in libFuzzer, even into a shared library. This seems to be the
  # cause of linker errors, so we fall back to linking statically here until the situation improves.
  # TODO: We may be able to craft our own fuzzer-no-link option by setting an appropriate /NODEFAULTLIB as a linker
  # flag on shared libraries only.
  add_library(parser parser.cpp)
else()
  add_library(parser SHARED parser.cpp)
endif()
target_sources(parser PUBLIC
               parser.h)
target_link_libraries(parser PRIVATE helper)
target_include_directories(parser PUBLIC ${CMAKE_CURRENT_SOURCE_DIR})

if(CIFUZZ_USE_DEPRECATED_MACROS)
  add_fuzz_test(legacy_fuzz_test legacy_fuzz_test.cpp)
else()
  add_fuzz_test(parser_fuzz_test parser_fuzz_test.cpp)
  target_link_libraries(parser_fuzz_test PRIVATE parser)

  add_fuzz_test(no_seed_corpus_fuzz_test crash_always_fuzz_target.cpp)

  add_fuzz_test(c_fuzz_test c_fuzz_test.c)
endif()
