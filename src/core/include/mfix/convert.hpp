#pragma once

#include "field.hpp"
#include <chrono>
#include <vector>

namespace mfix {
    struct Date: std::chrono::year_month_day {};

    struct Time {
        std::chrono::hours hr {}; 
        std::chrono::minutes mn {}; 
        std::chrono::seconds sc {}; 
        std::chrono::milliseconds ms {};

        [[nodiscard]] std::chrono::milliseconds count() const;
        explicit Time(std::chrono::milliseconds total);
    };

    struct TZTime: Time {
        std::chrono::minutes offset {};

        [[nodiscard]] std::chrono::milliseconds count() const;
        explicit TZTime(std::chrono::milliseconds total, 
                std::chrono::minutes off);
    };

    struct TZTimestamp { 
        Date date; TZTime time; 

        TZTimestamp(std::chrono::year_month_day ymd, 
            std::chrono::milliseconds total, 
            std::chrono::minutes off); 

        [[nodiscard]] std::chrono::system_clock::time_point 
        to_sys_time() const;
    };

    struct MonthYear {
        std::chrono::month month;
        std::chrono::year year;
        std::optional<std::chrono::day> day {};
        std::optional<unsigned> week {};
    };

    namespace convert {
        [[nodiscard]] std::optional<int64_t> to_int(const Field &);
        [[nodiscard]] std::optional<uint64_t> to_uint(const Field &);
        [[nodiscard]] std::optional<double> to_double(const Field &);
        [[nodiscard]] std::optional<char> to_char(const Field &);
        [[nodiscard]] std::optional<bool> to_bool(const Field &);

        [[nodiscard]] std::optional<std::vector<char>> 
        to_char_vector(const Field &);

        [[nodiscard]] std::optional<std::vector<std::string>> 
        to_str_vector(const Field &);

        [[nodiscard]] std::optional<Date> to_date(const Field &);
        [[nodiscard]] std::optional<Time> to_time(const Field &);
        [[nodiscard]] std::optional<TZTime> to_tztime(const Field &);
        [[nodiscard]] std::optional<MonthYear> to_monthyear(const Field &);
        [[nodiscard]] std::optional<TZTimestamp> to_tztimestamp(const Field &);
    }
}
