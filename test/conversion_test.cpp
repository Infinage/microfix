#include "mfix/converter.hpp"
#include <gtest/gtest.h>

using namespace std::chrono_literals;

TEST(FieldTest, GENERIC_STRING) {
    mfix::Field f{0, "Hello"};
    EXPECT_EQ(f.value, "Hello");
}

TEST(FieldTest, AMT) {
    mfix::Field f1 {0, "23.23"};
    auto r1 = mfix::convert::to_double(f1);
    ASSERT_TRUE(r1.has_value()); 
    EXPECT_EQ(r1.value(), 23.23);

    mfix::Field f2 {0, "0023.2300"};
    auto r2 = mfix::convert::to_double(f2);
    ASSERT_TRUE(r2.has_value()); 
    EXPECT_EQ(r2.value(), 23.23);

    mfix::Field f3 {0, "-23.23"};
    auto r3 = mfix::convert::to_double(f3);
    ASSERT_TRUE(r3.has_value()); 
    EXPECT_EQ(r3.value(), -23.23);

    mfix::Field f4 {0, "25"};
    auto r4 = mfix::convert::to_double(f4);
    ASSERT_TRUE(r4.has_value()); 
    EXPECT_EQ(r4.value(), 25);

    for (auto val: {"+23", "10.0a", "", ".", "23..", ".23."}) {
        mfix::Field f {0, val};
        auto r = mfix::convert::to_double(f);
        ASSERT_FALSE(r.has_value()) << "Failed for: " << val; 
    }
}

TEST(FieldTest, BOOLEAN) {
    mfix::Field f1{0, "Y"};
    auto r1 = mfix::convert::to_bool(f1);
    ASSERT_TRUE(r1.has_value()); 
    EXPECT_TRUE(r1.value());

    mfix::Field f2{0, "N"};
    auto r2 = mfix::convert::to_bool(f2);
    ASSERT_TRUE(r2.has_value()); 
    EXPECT_FALSE(r2.value());

    for (auto val: {"y", "n", "true", "t", "", "YN", "NY"}) {
        mfix::Field f{0, val};
        auto r = mfix::convert::to_bool(f);
        ASSERT_FALSE(r.has_value()) << "Failed for: " << val; 
    }
}

TEST(FieldTest, CHAR) {
    mfix::Field f1 {0, "a"};
    auto r1 = mfix::convert::to_char(f1);
    ASSERT_TRUE(r1.has_value());
    EXPECT_EQ(r1.value(), 'a');

    mfix::Field f2 {0, "!"};
    auto r2 = mfix::convert::to_char(f2);
    ASSERT_TRUE(r2.has_value());
    EXPECT_EQ(r2.value(), '!');

    for (auto val: {"ab", ""}) {
        mfix::Field f {0, val};
        auto r = mfix::convert::to_char(f);
        ASSERT_FALSE(r.has_value()) << "Failed for: " << val; 
    }
}

TEST(FieldTest, DATA) {
    using namespace std::string_literals;
    std::string raw = "\1\0\1\0\1\n\r"s;
    mfix::Field f1 {0, raw};
    auto &r1 = f1.value;
    EXPECT_EQ(r1.size(), 7);
    EXPECT_EQ(r1, raw);
}

TEST(FieldTest, INT) {
    mfix::Field f1 {0, "23"};
    auto r1 = mfix::convert::to_int(f1);
    ASSERT_TRUE(r1.has_value()); 
    EXPECT_EQ(r1.value(), 23);

    mfix::Field f2 {0, "00023"};
    auto r2 = mfix::convert::to_int(f2);
    ASSERT_TRUE(r2.has_value()); 
    EXPECT_EQ(r2.value(), 23);

    mfix::Field f3 {0, "-23"};
    auto r3 = mfix::convert::to_int(f3);
    ASSERT_TRUE(r3.has_value()); 
    EXPECT_EQ(r3.value(), -23);

    mfix::Field f4 {0, "-00023"};
    auto r4 = mfix::convert::to_int(f4);
    ASSERT_TRUE(r4.has_value()); 
    EXPECT_EQ(r4.value(), -23);

    for (auto val: {"+23", "23.0", "10a", "", " 23", "999999999999999999999"}) {
        mfix::Field f {0, val};
        auto r = mfix::convert::to_int(f);
        ASSERT_FALSE(r.has_value()) << "Failed for: " << val; 
    }
}

TEST(FieldTest, LENGTH) {
    mfix::Field f1 {0, "23"};
    auto r1 = mfix::convert::to_uint(f1);
    ASSERT_TRUE(r1.has_value()); 
    EXPECT_EQ(r1.value(), 23);

    mfix::Field f2 {0, "00023"};
    auto r2 = mfix::convert::to_uint(f2);
    ASSERT_TRUE(r2.has_value()); 
    EXPECT_EQ(r2.value(), 23);

    for (auto val: {"-1", "-0", "+23"}) {
        mfix::Field f {0, val};
        auto r = mfix::convert::to_uint(f);
        ASSERT_FALSE(r.has_value()) << "Failed for: " << val; 
    }
}

TEST(FieldTest, MULTIPLECHARVALUE) {
    mfix::Field f1 {0, "2 A F"};
    auto r1 = mfix::convert::to_char_vector(f1);
    ASSERT_TRUE(r1.has_value());
    ASSERT_EQ(r1->size(), 3);
    ASSERT_EQ(r1->at(0), '2');
    ASSERT_EQ(r1->at(1), 'A');
    ASSERT_EQ(r1->at(2), 'F');

    mfix::Field f2 {0, "2"};
    auto r2 = mfix::convert::to_char_vector(f2);
    ASSERT_TRUE(r1.has_value());
    ASSERT_EQ(r2->size(), 1);
    ASSERT_EQ(r2->at(0), '2');

    for (auto val: {"2 2", "2 AF", "", "A ", " A", " A "}) {
        mfix::Field f {0, val};
        auto r = mfix::convert::to_char_vector(f);
        ASSERT_FALSE(r.has_value()) << "Failed for: " << val; 
    }
}

TEST(FieldTest, MULTIPLESTRINGVALUE) {
    mfix::Field f1 {0, "2 A F"};
    auto r1 = mfix::convert::to_str_vector(f1);
    ASSERT_TRUE(r1.has_value());
    ASSERT_EQ(r1->size(), 3);
    ASSERT_EQ(r1->at(0), "2");
    ASSERT_EQ(r1->at(1), "A");
    ASSERT_EQ(r1->at(2), "F");

    mfix::Field f2 {0, "2A"};
    auto r2 = mfix::convert::to_str_vector(f2);
    ASSERT_TRUE(r2.has_value());
    ASSERT_EQ(r2->size(), 1);
    ASSERT_EQ(r2->at(0), "2A");

    for (auto val: {"AA AA", "", "A ", " A"}) {
        mfix::Field f {0, val};
        auto r = mfix::convert::to_char_vector(f);
        ASSERT_FALSE(r.has_value()) << "Failed for: " << val; 
    }
}

TEST(FieldTest, LOCALMKTDATE) {
    mfix::Field f1 {0, "20240101"};
    auto r1 = mfix::convert::to_date(f1);
    ASSERT_TRUE(r1.has_value());
    ASSERT_EQ(r1->day(), 1d);
    ASSERT_EQ(r1->month(), std::chrono::January);
    ASSERT_EQ(r1->year(), 2024y);

    auto tests = {"20241301",  "20241234",  "20241234a", "20241210 ", 
        "", "202401", "2024-01-01"};

    for (auto val: tests) {
        mfix::Field f {0, val};
        auto r = mfix::convert::to_date(f);
        ASSERT_FALSE(r.has_value()) << "Failed for: " << val; 
    }
}

TEST(FieldTest, LOCALMKTTIME) {
    mfix::Field f1 {0, "01:02:03"};
    auto r1 = mfix::convert::to_time(f1);
    ASSERT_TRUE(r1.has_value());
    ASSERT_EQ(r1->hr, 1h);
    ASSERT_EQ(r1->mn, 2min);
    ASSERT_EQ(r1->sc, 3s);

    auto tests = {"01:02:03.444", "", "1:2:3", "24:00:00", "00:60:00", "01:00:60"};
    for (auto val: tests) {
        mfix::Field f {0, val};
        auto r = mfix::convert::to_time(f);
        ASSERT_FALSE(r.has_value()) << "Failed for: " << val; 
    }
}

TEST(FieldTest, TZTIMEONLY) {
    // f1: Short format with Zulu
    mfix::Field f1 {0, "01:02Z"};
    auto r1 = mfix::convert::to_tztime(f1);
    ASSERT_TRUE(r1.has_value());
    EXPECT_EQ(r1->hr, 1h);
    EXPECT_EQ(r1->mn, 2min);
    EXPECT_EQ(r1->offset, 0min);

    // f2: Full format with Zulu
    mfix::Field f2 {0, "01:02:03Z"};
    auto r2 = mfix::convert::to_tztime(f2);
    ASSERT_TRUE(r2.has_value());
    EXPECT_EQ(r2->sc, 3s);
    EXPECT_EQ(r2->offset, 0min);

    // f3: Positive offset (hours only)
    mfix::Field f3 {0, "01:02:03+01"};
    auto r3 = mfix::convert::to_tztime(f3);
    ASSERT_TRUE(r3.has_value());
    EXPECT_EQ(r3->offset, 60min);
    EXPECT_EQ(r3->count(), 2min + 3s);

    // f4: Positive offset (hours and minutes)
    mfix::Field f4 {0, "01:02:03+01:00"};
    auto r4 = mfix::convert::to_tztime(f4);
    ASSERT_TRUE(r4.has_value());
    EXPECT_EQ(r4->offset, 60min);

    // f5: Negative offset
    mfix::Field f5 {0, "01:02:03-01:00"};
    auto r5 = mfix::convert::to_tztime(f5);
    ASSERT_TRUE(r5.has_value());
    EXPECT_EQ(r5->offset, -60min);
    EXPECT_EQ(r5->count(), 2h + 2min + 3s);

    // f6: Partial hour offset (The "Chennai" style test)
    mfix::Field f6 {0, "01:02:03+01:30"};
    auto r6 = mfix::convert::to_tztime(f6);
    ASSERT_TRUE(r6.has_value());
    EXPECT_EQ(r6->offset, 90min);

    auto tests = {"01:02:03", "01:02:03.444Z", "", "01:02Z+01", "01:02:03+13", 
            "01:02:03-13", "1:2:3+0160"};

    for (auto val: tests) {
        mfix::Field f {0, val};
        auto r = mfix::convert::to_tztime(f);
        ASSERT_FALSE(r.has_value()) << "Failed for: " << val; 
    }
}

TEST(FieldTest, MONTHYEAR) {
    // f1: Standard MonthYear
    mfix::Field f1 {0, "202401"};
    auto r1 = mfix::convert::to_monthyear(f1);
    ASSERT_TRUE(r1.has_value());
    EXPECT_EQ(r1->year, 2024y);
    EXPECT_EQ(r1->month, std::chrono::January);
    EXPECT_FALSE(r1->day.has_value());
    EXPECT_FALSE(r1->week.has_value());

    // f2: MonthYear with Day
    mfix::Field f2 {0, "20240130"};
    auto r2 = mfix::convert::to_monthyear(f2);
    ASSERT_TRUE(r2.has_value());
    EXPECT_EQ(r2->day, 30d);

    // f3: MonthYear with Week 1
    mfix::Field f3 {0, "202401w1"};
    auto r3 = mfix::convert::to_monthyear(f3);
    ASSERT_TRUE(r3.has_value());
    EXPECT_EQ(r3->week, 1);

    // f4: MonthYear with Week 5
    mfix::Field f4 {0, "202401w5"};
    auto r4 = mfix::convert::to_monthyear(f4);
    ASSERT_TRUE(r4.has_value());
    EXPECT_EQ(r4->week, 5);

    // f5: Invalid Month (13)
    mfix::Field f5 {0, "202413"};
    EXPECT_FALSE(mfix::convert::to_monthyear(f5).has_value());

    // f6: Invalid Week (w6)
    mfix::Field f6 {0, "202401w6"};
    EXPECT_FALSE(mfix::convert::to_monthyear(f6).has_value());

    // f7: Invalid Week (w0)
    mfix::Field f7 {0, "202401w0"};
    EXPECT_FALSE(mfix::convert::to_monthyear(f7).has_value());
}

TEST(FieldTest, TZTIMESTAMP) {
    using namespace std::chrono_literals;

    // f1: Zulu (UTC)
    mfix::Field f1 {0, "20060901-07:39:00Z"};
    auto r1 = mfix::convert::to_tztimestamp(f1);
    ASSERT_TRUE(r1.has_value());
    EXPECT_EQ(r1->date.year(), 2006y);
    EXPECT_EQ(r1->time.offset, 0min);

    // f2: Negative Offset
    mfix::Field f2 {0, "20060901-02:39:00-05"};
    auto r2 = mfix::convert::to_tztimestamp(f2);
    ASSERT_TRUE(r2.has_value());
    EXPECT_EQ(r2->time.offset, -300min); // -5 hours

    // f4: The "Chennai" style Offset (+05:30)
    mfix::Field f4 {0, "20060901-13:09:00+05:30"};
    auto r4 = mfix::convert::to_tztimestamp(f4);
    ASSERT_TRUE(r4.has_value());
    EXPECT_EQ(r4->time.offset, 330min); 

    // f5: With Milliseconds
    mfix::Field f5 {0, "20060901-13:09:00.123+05:30"};
    auto r5 = mfix::convert::to_tztimestamp(f5);
    ASSERT_TRUE(r5.has_value());
    EXPECT_EQ(r5->time.ms, 123ms);

    for (auto val: {"20060901-07:39:00", "", "20060901", "20060901T07:39:00Z"}) {
        mfix::Field f {0, val};
        auto r = mfix::convert::to_tztimestamp(f);
        ASSERT_FALSE(r.has_value()) << "Failed for: " << val; 
    }
}
