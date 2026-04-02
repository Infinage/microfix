#include "mfix/utils.hpp"
#include <gtest/gtest.h>

TEST(ChecksumTest, BasicAscii) {
    std::string_view message = "8=FIX.4.4"; 
    EXPECT_EQ(mfix::checksum(message), 32);
}

TEST(ChecksumTest, SeparatorHandling) {
    std::string_view message = "A=B\x01";
    EXPECT_EQ(mfix::checksum(message), 193);
}

TEST(ChecksumTest, ModuloWrap) {
    std::string_view message = "zzz"; 
    EXPECT_EQ(mfix::checksum(message), 110);
}

TEST(ChecksumTest, HighBitCharacters) {
    std::string message = "\xFF\xFF"; 
    EXPECT_EQ(mfix::checksum(message), 254);
}
