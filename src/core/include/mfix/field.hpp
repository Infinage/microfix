#pragma once
#include <chrono>
#include <string>
#include <optional>
#include <charconv>
#include <vector>

namespace mfix {
    struct Field {
        int tag;
        std::string value;

        template<typename T> [[nodiscard]] 
        std::optional<T> get() const {
            T result {};
            auto *beg = value.data(), *end = beg + value.size();
            auto [ptr, ec] = std::from_chars(beg, end, result);
            if (ec == std::errc{} && ptr == end) return result;
            return std::nullopt;
        }

    };

    struct Date: std::chrono::year_month_day {};

    struct Time {
        std::chrono::hours hr {}; 
        std::chrono::minutes mn {}; 
        std::chrono::seconds sc {}; 
        std::chrono::milliseconds ms {};

        explicit Time(std::chrono::milliseconds total) {
            hr = std::chrono::duration_cast<std::chrono::hours>(total);
            total -= hr;
            mn = std::chrono::duration_cast<std::chrono::minutes>(total);
            total -= mn;
            sc = std::chrono::duration_cast<std::chrono::seconds>(total);
            total -= sc;
            ms = total;
        }

        std::chrono::milliseconds count() const { 
            return hr + mn + sc + ms;
        }
    };

    struct TZTime: Time {
        std::chrono::minutes offset {};
        explicit TZTime(std::chrono::milliseconds total, std::chrono::minutes off):
            Time{total} { this->offset = off; };

        std::chrono::milliseconds count() const {
            return Time::count() - offset;
        }
    };

    struct TZTimestamp { 
        Date date; TZTime time; 

        TZTimestamp(
            std::chrono::year_month_day ymd, 
            std::chrono::milliseconds total, 
            std::chrono::minutes off): 
        date {ymd}, time{total, off} {};

        [[nodiscard]] std::chrono::system_clock::time_point 
        to_sys_time() const;
    };

    struct MonthYear {
        std::chrono::month month;
        std::chrono::year year;
        std::optional<std::chrono::day> day {};
        std::optional<std::chrono::weeks> week {};
    };

    template<> std::optional<std::string> Field::get() const;

    template<> std::optional<char> Field::get() const;

    template<> std::optional<bool> Field::get() const;

    template<> std::optional<std::vector<char>> Field::get() const;

    template<> std::optional<std::vector<std::string>> Field::get() const;

    // LOCALMKTDATE: 20240101
    template<> std::optional<Date> Field::get() const;

    // LOCALMKTTIME: 01:02:03
    template<> std::optional<Time> Field::get() const;

    // TZTIMEONLY: 07:39:00.123+05:30
    template<> std::optional<TZTime> Field::get() const;

    // TZTIMESTAMP: 20060901-13:09:00.123+05:30
    template<> std::optional<TZTimestamp> Field::get() const;

    // MONTHYEAR: 202401w5
    template<> std::optional<MonthYear> Field::get() const;
}
