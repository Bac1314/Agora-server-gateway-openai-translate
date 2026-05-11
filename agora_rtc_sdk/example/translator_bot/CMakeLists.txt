cmake_minimum_required(VERSION 2.4)
project(TranslatorBot)

find_package(PkgConfig REQUIRED)
pkg_check_modules(LWS REQUIRED libwebsockets)
pkg_check_modules(SRC REQUIRED samplerate)

file(GLOB BOT_SOURCES
    "${PROJECT_SOURCE_DIR}/translator_bot.cpp"
    "${PROJECT_SOURCE_DIR}/openai_ws_client.cpp"
    "${PROJECT_SOURCE_DIR}/audio_pipeline.cpp"
    "${PROJECT_SOURCE_DIR}/../common/*.cpp"
)

add_executable(translator_bot ${BOT_SOURCES})

target_include_directories(translator_bot PRIVATE
    ${PROJECT_SOURCE_DIR}
    ${CMAKE_SOURCE_DIR}/third-party/json_parser/include
    ${LWS_INCLUDE_DIRS}
    ${SRC_INCLUDE_DIRS}
)

target_link_libraries(translator_bot
    ${LWS_LIBRARIES}
    ${SRC_LIBRARIES}
    ssl
    crypto
)

target_compile_options(translator_bot PRIVATE
    ${LWS_CFLAGS_OTHER}
    ${SRC_CFLAGS_OTHER}
    -std=c++14
)
