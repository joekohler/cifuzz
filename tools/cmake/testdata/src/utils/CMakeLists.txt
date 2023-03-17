if(WIN32)
  # libfuzzer has to be linked statically for clang-cl to get fuzzer
  # instrumentation, but that causes the linker to link in libFuzzer, even into a shared library. This seems to be the
  # cause of linker errors, so we fall back to linking statically here until the situation improves.
  # TODO: We may be able to craft our own fuzzer-no-link option by setting an appropriate /NODEFAULTLIB as a linker
  # flag on shared libraries only.
    add_library(helper helper.cpp)
else()
    add_library(helper SHARED helper.cpp)
endif()
target_sources(helper PUBLIC
               helper.h)
target_link_libraries(helper PRIVATE secrets)
target_include_directories(helper PUBLIC ${CMAKE_CURRENT_SOURCE_DIR})
