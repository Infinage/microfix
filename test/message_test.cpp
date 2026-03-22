#include "mfix/message.hpp"
#include <gtest/gtest.h>

TEST(MessageTest, BasicOps) {
    mfix::Message message{
        {8, "FIX.4.4"},
        {35, "5"},
        {34, "1091"},
        {5000, "Dummy1"},
        {49, "TESTBUY1"},
        {52, "20180920-18:24:58.675"},
        {56, "TESTSELL1"},
        {5000, "Dummy2"},
    };

    ASSERT_EQ(message.size(), 8);
    ASSERT_TRUE(message.contains(35));
    ASSERT_FALSE(message.contains(999));

    auto code = message.code();
    ASSERT_TRUE(code.has_value());
    EXPECT_EQ(code.value(), "5");
}

TEST(MessageTest, EmptyMessage) {
    mfix::Message empty_msg({});
    EXPECT_EQ(empty_msg.size(), 0);
    EXPECT_EQ(empty_msg.find(35), nullptr);
    EXPECT_TRUE(empty_msg.findAll(35).empty());
}

TEST(MessageTest, SerializationOrdering) {
    mfix::Message msg{{5000, "1"}, {49, "A"}, {5000, "2"}};
    EXPECT_EQ(msg.to_string('|'), "5000=1|49=A|5000=2|");
}

TEST(MessageTest, EraseBoundaries) {
    mfix::Message msg{{1, "A"}, {2, "B"}};
    msg.erase(2);
    ASSERT_EQ(msg.size(), 1);
    EXPECT_FALSE(msg.contains(2));
    msg.erase(1);
    EXPECT_EQ(msg.size(), 0);
}

TEST(MessageTest, Find) {
    mfix::Message message{{49, "TESTBUY1"}, {56, "TESTSELL1"}};
    auto f = message.find(49);
    ASSERT_TRUE(f != nullptr);
    EXPECT_EQ(f->value, "TESTBUY1");
    f->value = "MODIFIED";
    EXPECT_EQ(message.find(49)->value, "MODIFIED");
    ASSERT_TRUE(message.find(999) == nullptr);
}

TEST(MessageTest, FindAll) {
    const mfix::Message message{{5000, "A"}, {5000, "B"}, {49, "X"}};
    auto result = message.findAll(5000);
    ASSERT_EQ(result.size(), 2);
    EXPECT_EQ(result[0]->value, "A");
    EXPECT_EQ(result[1]->value, "B");
}

TEST(MessageTest, PushBackAndBack) {
    mfix::Message message{{49, "A"}};
    message.push_back(50, "B");
    ASSERT_EQ(message.size(), 2);
    EXPECT_EQ(message.back().value, "B");
}

TEST(MessageTest, Erase) {
    mfix::Message message{{5000, "A"}, {5000, "B"}, {49, "X"}};
    auto removed = message.erase(5000);
    ASSERT_EQ(removed, 2);
    ASSERT_EQ(message.size(), 1);
    ASSERT_EQ(message.erase(999), 0);
}

TEST(MessageTest, Serialize) {
    mfix::Message message{{8, "FIX.4.4"}, {9, "63"}, {35, "5"}, {10, "138"}};
    EXPECT_EQ(message.to_string(), "8=FIX.4.4\0019=63\00135=5\00110=138\001");
    EXPECT_EQ(message.to_string('|'), "8=FIX.4.4|9=63|35=5|10=138|");
}

TEST(MessageTest, ConstCorrectness) {
    mfix::Message msg{{35, "D"}};
    const mfix::Message& c_msg = msg;

    // This should compile and return a const Field*
    const auto *f = c_msg.find(35);
    ASSERT_NE(f, nullptr);
    
    auto results = c_msg.findAll(35);
    using findAllType = decltype(results[0]);
    static_assert(std::is_const_v<std::remove_pointer_t<std::remove_reference_t<findAllType>>>);
}
