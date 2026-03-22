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

    const auto *f = c_msg.find(35);
    ASSERT_NE(f, nullptr);
    
    auto results = c_msg.findAll(35);
    using findAllType = decltype(results[0]);
    static_assert(std::is_const_v<std::remove_pointer_t<std::remove_reference_t<findAllType>>>);
}

TEST(MessageTest, FromString_Valid) {
    std::string_view raw = "8=FIX.4.4|9=63|35=A|10=123|";
    auto res = mfix::Message::from_string(raw, '|');
    
    ASSERT_TRUE(res.has_value()) << res.error();
    ASSERT_EQ(res->code(), "A");
    EXPECT_EQ(res->size(), 4);
    EXPECT_EQ(res->find(8)->value, "FIX.4.4");
}

TEST(MessageTest, FromString_EmptyInput) {
    auto res = mfix::Message::from_string("", '|');
    ASSERT_FALSE(res.has_value());
}

TEST(MessageTest, FromString_OnlySeparator) {
    auto res = mfix::Message::from_string("|", '|');
    ASSERT_FALSE(res.has_value());
}

TEST(MessageTest, FromString_EmptyTag) {
    auto res = mfix::Message::from_string("=FIX|9=12|", '|');
    ASSERT_FALSE(res.has_value());
}

TEST(MessageTest, FromString_EmptyValue) {
    std::string_view raw = "8=FIX.4.4|1=|35=A|";
    auto res = mfix::Message::from_string(raw, '|');

    ASSERT_TRUE(res.has_value()) << res.error();
    auto* f = res->find(1);
    ASSERT_NE(f, nullptr);
    EXPECT_TRUE(f->value.empty());
}

TEST(MessageTest, FromString_Malformed_NoEquals) {
    std::string_view raw = "8=FIX.4.4|INVALID_FIELD|35=A|";
    auto res = mfix::Message::from_string(raw, '|');

    ASSERT_FALSE(res.has_value());
    EXPECT_TRUE(res.error().starts_with("Must have exactly one '='"));
}

TEST(MessageTest, FromString_Malformed_BadTag) {
    std::string_view raw = "8=FIX.4.4|TAG_IS_NOT_INT=Value|";
    auto res = mfix::Message::from_string(raw, '|');
    
    ASSERT_FALSE(res.has_value());
    EXPECT_TRUE(res.error().starts_with("Tag not an INT"));
}

TEST(MessageTest, FromString_MultipleEquals) {
    std::string_view raw = "8=FIX.4.4|58=Data=With=Equals|";
    auto res = mfix::Message::from_string(raw, '|');
    ASSERT_FALSE(res.has_value());
}

TEST(MessageTest, FromString_EmptyField) {
    auto res = mfix::Message::from_string("8=FIX||9=12|", '|');
    ASSERT_FALSE(res.has_value());
}

TEST(MessageTest, FromString_Random) {
    auto testCases = {
        "8=FIX.4.49=14835=D34=108049=TESTBUY152=20180920-18:14:19.508"
        "56=TESTSELL111=63673064027889863415=USD21=238=700040=154=1"
        "55=MSFT60=20180920-18:14:19.49210=092",

        "8=FIX.4.49=7535=A34=109249=TESTBUY152=20180920-18:24:59.643"
            "56=TESTSELL198=0108=6010=178",

        "8=FIX.4.49=6335=534=109149=TESTBUY152=20180920-18:24:58.675"
            "56=TESTSELL110=138"
    };

    for (auto fixMsg: testCases) {
        auto res = mfix::Message::from_string(fixMsg);
        ASSERT_TRUE(res.has_value());
    }
}
