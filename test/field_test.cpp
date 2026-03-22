#include "mfix/field.hpp"
#include <gtest/gtest.h>

using namespace std::chrono_literals;

TEST(FieldTest, GENERIC_STRING) {
    mfix::Field f{.tag = 0, .value = "Hello"};
    auto r = f.get<std::string>();
    ASSERT_TRUE(r.has_value());
    EXPECT_EQ(r.value(), "Hello");
}

TEST(FieldTest, AMT) {
    mfix::Field f1 {.tag = 0, .value = "23.23"};
    auto r1 = f1.get<double>();
    ASSERT_TRUE(r1.has_value()); 
    EXPECT_EQ(r1.value(), 23.23);

    mfix::Field f2 {.tag = 0, .value = "0023.2300"};
    auto r2 = f2.get<double>();
    ASSERT_TRUE(r2.has_value()); 
    EXPECT_EQ(r2.value(), 23.23);

    mfix::Field f3 {.tag = 0, .value = "-23.23"};
    auto r3 = f3.get<double>();
    ASSERT_TRUE(r3.has_value()); 
    EXPECT_EQ(r3.value(), -23.23);

    mfix::Field f4 {.tag = 0, .value = "25"};
    auto r4 = f4.get<double>();
    ASSERT_TRUE(r4.has_value()); 
    EXPECT_EQ(r4.value(), 25);

    mfix::Field f5 {.tag = 0, .value = "+23"};
    auto r5 = f5.get<double>();
    ASSERT_FALSE(r5.has_value());

    mfix::Field f6 {.tag = 0, .value = "10.0a"};
    auto r6 = f6.get<double>();
    ASSERT_FALSE(r6.has_value());
}

TEST(FieldTest, BOOLEAN) {
    mfix::Field f1{.tag = 0, .value = "Y"};
    auto r1 = f1.get<bool>();
    ASSERT_TRUE(r1.has_value()); 
    EXPECT_TRUE(r1.value());

    mfix::Field f2{.tag = 0, .value = "N"};
    auto r2 = f2.get<bool>();
    ASSERT_TRUE(r2.has_value()); 
    EXPECT_FALSE(r2.value());

    for (auto val: {"y", "n", "true", "t"}) {
        mfix::Field f{.tag = 0, .value = val};
        auto r = f.get<bool>();
        ASSERT_FALSE(r.has_value());
    }
}

TEST(FieldTest, CHAR) {
    mfix::Field f1 {.tag = 0, .value = "a"};
    auto r1 = f1.get<char>();
    ASSERT_TRUE(r1.has_value());
    EXPECT_EQ(r1.value(), 'a');

    mfix::Field f2 {.tag = 0, .value = "!"};
    auto r2 = f2.get<char>();
    ASSERT_TRUE(r2.has_value());
    EXPECT_EQ(r2.value(), '!');

    mfix::Field f3 {.tag = 0, .value = "ab"};
    auto r3 = f3.get<char>();
    ASSERT_FALSE(r3.has_value());
}

TEST(FieldTest, DATA) {
    using namespace std::string_literals;
    std::string raw = "\1\0\1\0\1\n\r"s;
    mfix::Field f1 {.tag = 0, .value = raw};
    auto r1 = f1.get<std::string>();
    ASSERT_TRUE(r1.has_value());
    EXPECT_EQ(r1.value().size(), 7);
    EXPECT_EQ(r1.value(), raw);
}

TEST(FieldTest, INT) {
    mfix::Field f1 {.tag = 0, .value = "23"};
    auto r1 = f1.get<int>();
    ASSERT_TRUE(r1.has_value()); 
    EXPECT_EQ(r1.value(), 23);

    mfix::Field f2 {.tag = 0, .value = "00023"};
    auto r2 = f2.get<int>();
    ASSERT_TRUE(r2.has_value()); 
    EXPECT_EQ(r2.value(), 23);

    mfix::Field f3 {.tag = 0, .value = "-23"};
    auto r3 = f3.get<int>();
    ASSERT_TRUE(r3.has_value()); 
    EXPECT_EQ(r3.value(), -23);

    mfix::Field f4 {.tag = 0, .value = "-00023"};
    auto r4 = f4.get<int>();
    ASSERT_TRUE(r4.has_value()); 
    EXPECT_EQ(r4.value(), -23);

    for (auto val: {"+23", "23.0", "10a"}) {
        mfix::Field f {.tag = 0, .value = val};
        auto r = f.get<int>();
        ASSERT_FALSE(r.has_value()); 
    }
}

TEST(FieldTest, LENGTH) {
    mfix::Field f1 {.tag = 0, .value = "23"};
    auto r1 = f1.get<std::size_t>();
    ASSERT_TRUE(r1.has_value()); 
    EXPECT_EQ(r1.value(), 23);

    mfix::Field f2 {.tag = 0, .value = "00023"};
    auto r2 = f2.get<std::size_t>();
    ASSERT_TRUE(r2.has_value()); 
    EXPECT_EQ(r2.value(), 23);

    mfix::Field f3 {.tag = 0, .value = "-1"};
    auto r3 = f3.get<std::size_t>();
    ASSERT_FALSE(r3.has_value()); 
}

TEST(FieldTest, MULTIPLECHARVALUE) {
    mfix::Field f1 {.tag = 0, .value = "2 A F"};
    auto r1 = f1.get<std::vector<char>>();
    ASSERT_TRUE(r1.has_value());
    ASSERT_EQ(r1->size(), 3);
    ASSERT_EQ(r1->at(0), '2');
    ASSERT_EQ(r1->at(1), 'A');
    ASSERT_EQ(r1->at(2), 'F');

    mfix::Field f2 {.tag = 0, .value = "2"};
    auto r2 = f2.get<std::vector<char>>();
    ASSERT_TRUE(r1.has_value());
    ASSERT_EQ(r2->size(), 1);
    ASSERT_EQ(r2->at(0), '2');

    mfix::Field f3 {.tag = 0, .value = "2 2"};
    auto r3 = f3.get<std::vector<char>>();
    ASSERT_FALSE(r3.has_value());

    mfix::Field f4 {.tag = 0, .value = "2 AF"};
    auto r4 = f4.get<std::vector<char>>();
    ASSERT_FALSE(r4.has_value());
}

TEST(FieldTest, MULTIPLESTRINGVALUE) {
    mfix::Field f1 {.tag = 0, .value = "2 A F"};
    auto r1 = f1.get<std::vector<std::string>>();
    ASSERT_TRUE(r1.has_value());
    ASSERT_EQ(r1->size(), 3);
    ASSERT_EQ(r1->at(0), "2");
    ASSERT_EQ(r1->at(1), "A");
    ASSERT_EQ(r1->at(2), "F");

    mfix::Field f2 {.tag = 0, .value = "2A"};
    auto r2 = f2.get<std::vector<std::string>>();
    ASSERT_TRUE(r2.has_value());
    ASSERT_EQ(r2->size(), 1);
    ASSERT_EQ(r2->at(0), "2A");

    mfix::Field f3 {.tag = 0, .value = "AA AA"};
    auto r3 = f3.get<std::vector<std::string>>();
    ASSERT_FALSE(r3.has_value());
}

TEST(FieldTest, LOCALMKTDATE) {
    mfix::Field f1 {.tag = 0, .value = "20240101"};
    auto r1 = f1.get<mfix::Date>();
    ASSERT_TRUE(r1.has_value());
    ASSERT_EQ(r1->day(), 1d);
    ASSERT_EQ(r1->month(), std::chrono::January);
    ASSERT_EQ(r1->year(), 2024y);

    for (auto val: {"20241301",  "20241234",  "20241234a"}) {
        mfix::Field f {.tag = 0, .value = val};
        auto r = f.get<mfix::Date>();
        ASSERT_FALSE(r.has_value());
    }
}

TEST(FieldTest, LOCALMKTTIME) {
    mfix::Field f1 {.tag = 0, .value = "01:02:03"};
    auto r1 = f1.get<mfix::Time>();
    ASSERT_TRUE(r1.has_value());
    ASSERT_EQ(r1->hr, 1h);
    ASSERT_EQ(r1->mn, 2min);
    ASSERT_EQ(r1->sc, 3s);

    mfix::Field f2 {.tag = 0, .value = "01:02:03.444"};
    auto r2 = f2.get<mfix::Time>();
    ASSERT_FALSE(r2.has_value());
}

TEST(FieldTest, TZTIMEONLY) {
    // f1: Short format with Zulu
    mfix::Field f1 {.tag = 0, .value = "01:02Z"};
    auto r1 = f1.get<mfix::TZTime>();
    ASSERT_TRUE(r1.has_value());
    EXPECT_EQ(r1->hr, 1h);
    EXPECT_EQ(r1->mn, 2min);
    EXPECT_EQ(r1->offset, 0min);

    // f2: Full format with Zulu
    mfix::Field f2 {.tag = 0, .value = "01:02:03Z"};
    auto r2 = f2.get<mfix::TZTime>();
    ASSERT_TRUE(r2.has_value());
    EXPECT_EQ(r2->sc, 3s);
    EXPECT_EQ(r2->offset, 0min);

    // f3: Positive offset (hours only)
    mfix::Field f3 {.tag = 0, .value = "01:02:03+01"};
    auto r3 = f3.get<mfix::TZTime>();
    ASSERT_TRUE(r3.has_value());
    EXPECT_EQ(r3->offset, 60min);
    EXPECT_EQ(r3->count(), 2min + 3s);

    // f4: Positive offset (hours and minutes)
    mfix::Field f4 {.tag = 0, .value = "01:02:03+01:00"};
    auto r4 = f4.get<mfix::TZTime>();
    ASSERT_TRUE(r4.has_value());
    EXPECT_EQ(r4->offset, 60min);

    // f5: Negative offset
    mfix::Field f5 {.tag = 0, .value = "01:02:03-01:00"};
    auto r5 = f5.get<mfix::TZTime>();
    ASSERT_TRUE(r5.has_value());
    EXPECT_EQ(r5->offset, -60min);
    EXPECT_EQ(r5->count(), 2h + 2min + 3s);

    // f6: Partial hour offset (The "Chennai" style test)
    mfix::Field f6 {.tag = 0, .value = "01:02:03+01:30"};
    auto r6 = f6.get<mfix::TZTime>();
    ASSERT_TRUE(r6.has_value());
    EXPECT_EQ(r6->offset, 90min);

    // f7: Missing Offset (Should Fail for TZTime)
    mfix::Field f7 {.tag = 0, .value = "01:02:03"};
    auto r7 = f7.get<mfix::TZTime>();
    EXPECT_FALSE(r7.has_value()) << "TZTime should require an offset suffix";

    mfix::Field f8 {.tag = 0, .value = "01:02:03.444Z"};
    auto r8 = f8.get<mfix::Time>();
    ASSERT_FALSE(r8.has_value());
}

TEST(FieldTest, TZTIMESTAMP) {
    using namespace std::chrono_literals;

    // f1: Zulu (UTC)
    mfix::Field f1 {.tag = 0, .value = "20060901-07:39:00Z"};
    auto r1 = f1.get<mfix::TZTimestamp>();
    ASSERT_TRUE(r1.has_value());
    EXPECT_EQ(r1->date.year(), 2006y);
    EXPECT_EQ(r1->time.offset, 0min);

    // f2: Negative Offset
    mfix::Field f2 {.tag = 0, .value = "20060901-02:39:00-05"};
    auto r2 = f2.get<mfix::TZTimestamp>();
    ASSERT_TRUE(r2.has_value());
    EXPECT_EQ(r2->time.offset, -300min); // -5 hours

    // f4: The "Chennai" style Offset (+05:30)
    mfix::Field f4 {.tag = 0, .value = "20060901-13:09:00+05:30"};
    auto r4 = f4.get<mfix::TZTimestamp>();
    ASSERT_TRUE(r4.has_value());
    EXPECT_EQ(r4->time.offset, 330min); 

    // f5: With Milliseconds
    mfix::Field f5 {.tag = 0, .value = "20060901-13:09:00.123+05:30"};
    auto r5 = f5.get<mfix::TZTimestamp>();
    ASSERT_TRUE(r5.has_value());
    EXPECT_EQ(r5->time.ms, 123ms);

    // f6: Mandatory Timezone Missing -> Should Fail
    mfix::Field f6 {.tag = 0, .value = "20060901-07:39:00"};
    auto r6 = f6.get<mfix::TZTimestamp>();
    EXPECT_FALSE(r6.has_value());
}

TEST(FieldTest, MONTHYEAR) {
    // f1: Standard MonthYear
    mfix::Field f1 {.tag = 0, .value = "202401"};
    auto r1 = f1.get<mfix::MonthYear>();
    ASSERT_TRUE(r1.has_value());
    EXPECT_EQ(r1->year, 2024y);
    EXPECT_EQ(r1->month, std::chrono::January);
    EXPECT_FALSE(r1->day.has_value());
    EXPECT_FALSE(r1->week.has_value());

    // f2: MonthYear with Day
    mfix::Field f2 {.tag = 0, .value = "20240130"};
    auto r2 = f2.get<mfix::MonthYear>();
    ASSERT_TRUE(r2.has_value());
    EXPECT_EQ(r2->day, 30d);

    // f3: MonthYear with Week 1
    mfix::Field f3 {.tag = 0, .value = "202401w1"};
    auto r3 = f3.get<mfix::MonthYear>();
    ASSERT_TRUE(r3.has_value());
    EXPECT_EQ(r3->week, std::chrono::weeks{1});

    // f4: MonthYear with Week 5
    mfix::Field f4 {.tag = 0, .value = "202401w5"};
    auto r4 = f4.get<mfix::MonthYear>();
    ASSERT_TRUE(r4.has_value());
    EXPECT_EQ(r4->week, std::chrono::weeks{5});

    // f5: Invalid Month (13)
    mfix::Field f5 {.tag = 0, .value = "202413"};
    EXPECT_FALSE(f5.get<mfix::MonthYear>().has_value());

    // f6: Invalid Week (w6)
    mfix::Field f6 {.tag = 0, .value = "202401w6"};
    EXPECT_FALSE(f6.get<mfix::MonthYear>().has_value());
}
